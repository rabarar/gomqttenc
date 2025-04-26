
MESH_PROTO=../protobufs

gomqttenc: go.mod main.go parse_position.go parse_telemetry.go parser.go parse_map_report.go utils.go
	go mod tidy; go build

clean:
	rm gomqttenc
