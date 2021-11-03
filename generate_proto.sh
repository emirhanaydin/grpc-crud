IMPORT_PATH="."
SOURCES=("protobuf/*.proto")

for i in "${SOURCES[@]}"
do
  protoc -I="$IMPORT_PATH" --go_out=. "$i"
  protoc -I="$IMPORT_PATH" --go-grpc_out=. "$i"
done
