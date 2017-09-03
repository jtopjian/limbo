# Limbo - LXD Image Import / Export

Limbo is a tool to help import and export LXD images to/from different storage
backends.

## Storage Backends

* OpenStack Swift

## Usage

### Export

The most basic use of Limbo is:

```shell
$ limbo export swift --name foo --stop
```

This will temporarily stop (`--stop`) a running container called `foo` and
export it to a Swift storage container called `limbo`. The Swift container
`limbo` must exist first.

To have Limbo automatically create the storage container, do:

```shell
$ limbo export swift --name foo --stop --create-storage-container
```

To specify a custom storage container name, do:

```shell
$ limbo export swift --name foo --stop --create-storage-container --storage-container backups
```

To take advantage of Swift Object Versioning/Archiving, do:

```shell
$ limbo export swift --name foo --stop --create-storage-container --storage-container backups --archive
```

If you already have an image, do:

```shell
$ limbo export swift --name foo-image --type image --create-storage-container
```

Limbo can encrypt an image.

> Warning: I am not a crypto expert. I make no guarantees about the integrity
> of this feature.

```shell
$ limbo export swift --name foo --stop --encrypt --pass "some passphrase"
```

### Import

Importing an image works much the same way as exporting, but the data goes in
the opposite direction.

To import an image:

```shell
$ limbo import swift --object-name foo
```

This will look for an object named `foo` in a Swift storage container called
`limbo` and import it into LXD as `foo`.

To specify an alternative storage container name and LXD image name, do:

```shell
$ limbo import swift --storage-container backups --object-name foo --name foobar
```

To decrypt a previously encrypted image, do:

```shell
$ limbo import swift --object-name foo --encrypt --pass "some passphrase"
```

## Contributing

Any type of contribution is welcomed: documentation, bug reports, bug fixes,
and features.

## Development

Use a suitable Go-based development environment. More details soon.

## Installing from Source

```shell
$ go get -u github.com/jtopjian/limbo
$ go build -o ~/limbo ./
```
