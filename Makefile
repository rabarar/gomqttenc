
MESH_PROTO=../protobufs

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
	aesccm/main.go
	go mod tidy; go build

clean:
	rm gomqttenc
