
MESH_PROTO=../protobufs

all: gomqttenc plugins lint 

plugins: msh_plugin rtl433_plugin udp_plugin

msh_plugin: plugins/msh/msh_plugin.go \
   	shared/shared.go

	go build -buildmode=plugin -o plugins/msh.so gomqttenc/plugins/msh

rtl433_plugin: plugins/rtl433/rtl433_plugin.go \
   	shared/shared.go

	go build -buildmode=plugin -o plugins/rtl433.so gomqttenc/plugins/rtl433

udp_plugin: plugins/udp/udp_plugin.go \
   	shared/shared.go

	go build -buildmode=plugin -o plugins/udp.so gomqttenc/plugins/udp

gomqttenc: go.mod \
	md/decrypt.go  \
	aes.go \
	decode_local.go \
	errors.go \
	main.go \
	types.go \
	mqtt_handlers.go \
	plugin_manager.go \
	telegraf_pub.go \
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
	rm -rf *.so

