
MESH_PROTO=../protobufs

all: gomqttenc

gomqttenc: go.mod \
	aes.go \
	decode_local.go \
	errors.go \
	main.go \
	parse_map_report.go \
	parse_position.go \
	parse_telemetry.go \
	parser.go \
	utils.go \
	md/decrypt.go \
	aesccm/main.go
	go mod tidy; go build

clean:
	rm -f gomqttenc
