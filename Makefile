
MESH_PROTO=../protobufs

all: gomqttenc lint

gomqttenc: go.mod \
	md/decrypt.go  \
	aes.go \
	decode_local.go \
	errors.go \
	main.go \
	parse_map_report.go \
	parse_nodeinfo_report.go \
	parse_position.go \
	parse_rtl433.go \
	parse_telemetry.go \
	parse_text.go \
	parse.go \
	utils.go

	go mod tidy; go build

lint:
	 golangci-lint run

clean:
	rm -f gomqttenc
