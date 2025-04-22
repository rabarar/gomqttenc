# gomqttenc
golang MQTT Meshtastic decryption client

reference:
 https://buf.build/meshtastic/protobufs/docs/main:meshtastic

install
 `brew install protobuf protoc-gen-go`

protobuf generation:
 `protoc --go_out=./go $(find meshtastic -name '*.proto')`
 
