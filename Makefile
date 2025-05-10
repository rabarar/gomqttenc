
MESH_PROTO=../protobufs

all: gomqttenc lint

gomqttenc: go.mod \
	aes.go \
	decode_local.go \
	errors.go \
	main.go \
	parse_map_report.go \
	parse_position.go \
	parse_telemetry.go \
	utils.go \
	md/decrypt.go 
	go mod tidy; go build

lint:
	 golangci-lint run

clean:
	rm -f gomqttenc
