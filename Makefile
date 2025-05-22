
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
	md/*.go  \
	main.go \
	mqtt_handlers.go \
	plugin_manager.go \
	telegraf_pub.go \
	parser/*.go \
	rtl433/*.go \
	utils/*.go \

	go mod tidy; go build

lint:
	 golangci-lint run

clean:
	rm -f gomqttenc
	rm -rf *.so

