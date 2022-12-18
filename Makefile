all: build

build:
	go build

install: build
	mkdir -p /etc/scw-wau
	cp pn.yaml /etc/scw-wau/
	cp scw-wau /usr/local/bin/
	scw-wau install

remove:
	- scw-wau remove
	- rm -rf /etc/scw-wau /usr/local/bin/scw-wau

clean:
	- go clean
	- rm -f scw-wau

test: build
	./scw-wau -c pn.yaml.example
