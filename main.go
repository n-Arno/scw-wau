package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/vishvananda/netlink"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type Pn struct {
	Id string `yaml:"id"`
	Ip string `yaml:"ip"`
}

type Config struct {
	Pns    []Pn     `yaml:"pns"`
	Routes []string `yaml:"routes"`
}

func readConfig(file string) Config {
	content, err := ioutil.ReadFile(file)
	check(err)
	config := Config{}
	err = yaml.Unmarshal([]byte(content), &config)
	check(err)
	return config
}

type nic struct {
	id  string
	mac string
}

func isEqualNic(a nic, b nic) bool {
	return (a.id == b.id) && (a.mac == b.mac)
}

func isEqualNics(aa []nic, bb []nic) bool {
	if len(aa) != len(bb) {
		return false
	}
	for _, a := range aa {
		found := false
		for _, b := range bb {
			if isEqualNic(a, b) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

const metadataIp = "169.254.42.42"

func getNics() ([]nic, error) {
	nics := []nic{}
	metadataUrl := fmt.Sprintf("http://%v/conf?format=json", metadataIp)
	req, err := http.NewRequest(http.MethodGet, metadataUrl, nil)
	if err != nil {
		return nics, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nics, err
	}
	if res.StatusCode != 200 {
		errorString := fmt.Sprintf("Got HTTP %v from metadata api", res.StatusCode)
		return nics, errors.New(errorString)
	}

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nics, err
	}

	var result map[string]interface{}
	json.Unmarshal([]byte(resBody), &result)

	pns := result["private_nics"].([]interface{})
	for _, pn := range pns {
		n := pn.(map[string]interface{})
		nics = append(nics, nic{id: n["private_network_id"].(string), mac: n["mac_address"].(string)})
	}

	return nics, nil
}

type item struct {
	mu  sync.Mutex
	val []nic
}

type fn func(Config, *item, chan []nic)

func watch(config Config, current *item, event chan []nic) {
	(*current).mu.Lock() // lock to check if not already locked
	defer (*current).mu.Unlock()
	nics, err := getNics()
	if err == nil && !isEqualNics((*current).val, nics) {
		fmt.Printf("New private nics state: %v\n", nics)
		event <- nics // send only if changed
	}
}

func updateNic(nic string, ip string) {
	link, err := netlink.LinkByName(nic)
	check(err)
	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	check(err)
	for _, addr := range addrs {
		netlink.AddrDel(link, &addr)
	}
	addr, err := netlink.ParseAddr(ip)
	check(err)
	err = netlink.LinkSetUp(link)
	check(err)
	err = netlink.AddrAdd(link, addr)
	check(err)
}

func findNicIndex(ip net.IP) (int, error) {
	ifaces, err := net.Interfaces()
	check(err)
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		check(err)
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPNet:
				if v.IP.To4() != nil && !v.IP.IsLoopback() && v.Contains(ip) {
					return i.Index, nil
				}
			}
		}
	}
	return -1, errors.New("network not found")
}

func updateRoute(dest string, gw string) {
	_, dst, err := net.ParseCIDR(dest)
	check(err)
	ip := net.ParseIP(gw)
	linkIndex, err := findNicIndex(ip)
	if err == nil {
		link, err := netlink.LinkByIndex(linkIndex)
		check(err)
		err = netlink.LinkSetUp(link)
		check(err)
		route := netlink.Route{LinkIndex: linkIndex, Dst: dst, Gw: ip}
		err = netlink.RouteAppend(&route)
		check(err)
	}
}

func update(config Config, current *item, event chan []nic) {
	value := <-event     // block until receiving a value from watch
	(*current).mu.Lock() // lock during update
	defer (*current).mu.Unlock()
	ifaces, err := net.Interfaces()
	check(err)
	pns := make(map[string]string)
	for _, iface := range ifaces {
		if bytes.Compare(iface.HardwareAddr, nil) != 0 {
			for _, nic := range value {
				if strings.ToLower(nic.mac) == strings.ToLower(iface.HardwareAddr.String()) {
					fmt.Printf("Found interface %v with mac address %v\n", iface.Name, nic.mac)
					for _, pn := range config.Pns {
						if pn.Id == nic.id {
							pns[iface.Name] = pn.Ip
						}
					}
				}
			}
		}
	}
	for nic, ip := range pns {
		updateNic(nic, ip)
	}
	routes := make(map[string]string)
	r := regexp.MustCompile(`^(?P<Dest>default|[0-9\.\/]+)[ ]+via[ ]+(?P<Gw>[0-9\.]+)$`)
	for _, route := range config.Routes {
		m := r.FindStringSubmatch(route)
		if len(m) > 0 {
			routes[m[r.SubexpIndex("Dest")]] = m[r.SubexpIndex("Gw")]
		}
	}
	for dest, gw := range routes {
		updateRoute(dest, gw)
	}
	(*current).val = value // update once done
}

func loop(f fn, s int, config Config, current *item, event chan []nic) {
	for true {
		f(config, current, event)
		time.Sleep(time.Second * time.Duration(s))
	}
}

var (
	pool = flag.Int("p", 10, "pooling time")
	conf = flag.String("c", "pn.yaml", "config filename")
)

func main() {
	flag.Parse()
	config := readConfig(*conf)
	fmt.Printf("Starting pooling every %v seconds\n", *pool)

	curr := item{val: []nic{}}
	ev := make(chan []nic)
	go loop(watch, *pool, config, &curr, ev)
	go loop(update, 0, config, &curr, ev)

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	fmt.Printf("\nSignal (%v) received, stopping\n", s)
}
