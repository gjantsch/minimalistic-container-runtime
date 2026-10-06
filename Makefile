# container make file
# DOCKER_IMAGE_NAME=container-runtime

build-local:
	go build -o container .

build: 
	docker build -t container-runtime .

run:
	docker run --privileged --rm container-runtime ${ARGS}

clean:
	docker rmi container-runtime
