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


## notes

refactoring for multiple topics
    - modify json to make it an array of topics/QoS values
    - modify the subscribe -> subscribeMultiple 
```
        token := client.SubscribeMultiple(topics, messageHandler)
        token.Wait()
        if token.Error() != nil {
            log.Fatal(token.Error())
        }
```
and in handler:
```
switch msg.Topic() {
    case "/gps/node/data":
        handleGPS(msg)
    case "/sensor/temp":
        handleTemp(msg)
    }
```
where the handler has the following signatures: `func(client mqtt.Client, msg mqtt.Message)`

## Generic channels
```
package main

import (
	"fmt"
)

func main() {
	ch := make(chan interface{})

	// Start a goroutine to send values
	go func() {
		ch <- 42
		ch <- "hello"
		ch <- 3.14
		close(ch)
	}()

	// Receive and type switch
	for val := range ch {
		switch v := val.(type) {
		case int:
			fmt.Println("int:", v)
		case string:
			fmt.Println("string:", v)
		case float64:
			fmt.Println("float64:", v)
		default:
			fmt.Println("unknown type")
		}
	}
}
```
