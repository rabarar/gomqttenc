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

modify messageHandler to test for JSON 

   if isLikelyJSON(payload) {
        var result map[string]interface{}
        if err := json.Unmarshal(payload, &result); err != nil {
            log.Printf("Invalid JSON: %v", err)
            return
        }
        log.Printf("Got JSON: %+v", result)
    } else {
        // Assume it's Protobuf — try to decode it
   ``

func isLikelyJSON(payload []byte) bool {
    // Trim leading whitespace and check if the first non-space char is '{'
    for _, b := range payload {
        if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
            continue
        }
        return b == '{'
    }
    return false
}

