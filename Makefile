all: build

build:
	go build

install: build
	mkdir -p /etc/scw-wau
	cp pn.yaml /etc/scw-wau/
	cp scw-wau /usr/local/bin/
	scw-wau install

clean:
	- go clean
	- rm -f scw-wau

test: build
	./scw-wau -c pn.yaml.example
