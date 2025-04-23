
MESH_PROTO=../protobufs

gomqttenc: main.go 
	go mod tidy; go build

clean:
	rm gomqttenc
