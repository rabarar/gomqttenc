
MESH_PROTO=../protobufs
GENDIR=./go

gomqttenc: main.go 
	go mod tidy; go build


go-proto: 
	(cd ${MESH_PROTO}; rm -rf ${GENDIR}; mkdir ${GENDIR}; protoc --go_out=${GENDIR}  `find . -name '*.proto'`) && (touch mesh-proto-gen.txt)
	(rm -rf meshtastic) && (echo "removing old meshtastic protobuf gogen")
	(cp -rp ${MESH_PROTO}/${GENDIR}/github.com .) && (echo "copying new meshtastic package into tree")

clean:
	(rm -f gomqttenc; rm -rf github.com) && (echo "cleaned up executable and protobuf go package")
