# container make file

CMD  ?= /bin/sh
MEM  ?= 0
CPU  ?= 0

build-local:
	go build -o container .

build:
	docker build -t container-runtime .

run:
	docker run --privileged --cgroupns=host --rm container-runtime run --mem=$(MEM) --cpu=$(CPU) $(CMD)

clean:
	docker rmi container-runtime
