# gomqttenc
golang MQTT Meshtastic decryption client

## the goal is to generate a client that can decrypt incoming topic messages for further pipeline processing (i.e. grafana etc)

## Tools
Before you begin, you'll need `golang` installed as well as `protoc` and `protoc-gen-go` (using `brew` on OSX)
```
$brew install protobuf protoc-gen-go
```

## Protobuf Golang Package Generation
you can manually generate the `golang` protobufs as a stand-alone
```
cd protobufs  #cloned from https://github.com/meshtastic/protobufs
protoc --go_out=./go $(find meshtastic -name '*.proto')
```

## Simple Building using `make`
```
$ make
```

## Reference
 https://buf.build/meshtastic/protobufs/docs/main:meshtastic


 
