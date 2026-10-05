# container make file
DOCKER_IMAGE_NAME=container-runtime

build-local:
	go build -o container .

build: 
	docker build -t $(DOCKER_IMAGE_NAME) .

run:
	docker run --rm $(DOCKER_IMAGE_NAME)

clean:
	docker rmi --privileged $(DOCKER_IMAGE_NAME)
