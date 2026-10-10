# Minimal Container Runtime

This project is a challenge imposed to my self: build a minimalistic container runtime in order to better understand the big ones like [gVisor runsc](https://github.com/google/gvisor), [runc](https://github.com/opencontainers/runc) and [kata runtime](https://github.com/kata-containers/runtime).

Although I didn't studied none of them so far, the main motivation to start to write my own container runtime was to understand how does it feel in the perspective of any executable to be inside a container. And from the container perspective, what is needed and which tools the system have to create an isolated environment.

For short, is all about to understand the basic concepts behind this kind of tool and not to build something production ready.

In this journey, I had to fill many gaps of knowledge since I never studied operating systems in depth.

I choose to use the [syscall](https://pkg.go.dev/syscall) package instead of the [unix](https://pkg.go.dev/golang.org/x/sys/unix) just for the sake of simplicity and to  use less code possible. It also give an oportunity to create a second challenge with the unix package and understand the differences.

Full definition [here](./docs/CHALLENGE.md).

## Building & Running

```sh
make build && make run ARGS="/bin/ps"
```

## References

[Containers from Scratch by Liz Rice](https://www.youtube.com/watch?v=8fi7uSYlOdc) is an amazing place to start. In 40min you will have an wonderfull class exposing what I did here. Unfortunately I just found this video two weeks before I started to write this project.

[Linux Namespaces by Michael Kerrisk](https://www.youtube.com/watch?v=0kJPa-1FuoI) and [serveral articles about namespaces](https://lwn.net/Kernel/Index/#Namespaces-User_namespaces). Michael Kerrisk is the maintainer of the linux man pages and author of the **The Linux Programming Interface** book.

[How debuggers work by Eli Bendersky](https://eli.thegreenplace.net/2011/01/23/how-debuggers-work-part-1) is also a worth reading series of articles.
