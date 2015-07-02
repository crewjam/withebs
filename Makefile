
.PHONY: _withebs lint

all: withebs

withebs: withebs.go
	docker run -v $(PWD):/go/src/github.com/crewjam/withebs golang \
		make -C /go/src/github.com/crewjam/withebs _withebs

_withebs:
	go get ./...
	CGO_ENABLED=0 go install -a -installsuffix cgo -ldflags '-s' .
	ldd /go/bin/withebs | grep "not a dynamic executable"
	install /go/bin/withebs withebs

lint:
	go fmt ./...
	goimports -w *.go
