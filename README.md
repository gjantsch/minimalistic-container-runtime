# Minimal Container Runtime

This project is a challenge imposed to my self: build a minimalistic container runtime. 

The ideia is just to understand the basic concepts behind this kind of tool and not to build something production ready.

I choose to use the [syscall](https://pkg.go.dev/syscall) package instead of the [unix](https://pkg.go.dev/golang.org/x/sys/unix) for two basic reasons:
- First: use less code possible.
- Second: create a second challenge with the unix package and understand the differences.

Full definition [here](./docs/CHALLENGE.md).

## Building & Running

```sh
make build && make run ARGS="/bin/ps"
```
