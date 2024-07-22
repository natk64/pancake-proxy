package reflection

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ReflectionClient struct {
	mu            sync.Mutex
	v1Client      grpc_reflection_v1.ServerReflectionClient
	v1alphaClient grpc_reflection_v1alpha.ServerReflectionClient

	v1      v1ReflectionStream
	v1alpha v1alphaReflectionStream

	receivedFiles map[string]protoreflect.FileDescriptor
}

func (client *ReflectionClient) ListServices() ([]string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	stream, err := client.getStream()
	if err != nil {
		return nil, err
	}

	return stream.listServices()
}

func (client *ReflectionClient) AllFilesForSymbol(fullName string) ([]protoreflect.FileDescriptor, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	stream, err := client.getStream()
	if err != nil {
		return nil, err
	}

	encodedDescriptors, err := stream.fileContainingSymbol(fullName)
	if err != nil {
		return nil, err
	}

	fmt.Println(encodedDescriptors)
	return nil, fmt.Errorf("not implemented")
}

func (client *ReflectionClient) getStream() (reflectionStream, error) {
	if client.v1.client != nil {
		return client.v1, nil
	}

	stream, err := client.v1Client.ServerReflectionInfo(context.Background())
	if err != nil {
		return nil, err
	}

	client.v1.client = stream
	return client.v1, nil
}

func NewClient(conn *grpc.ClientConn) *ReflectionClient {
	client := &ReflectionClient{
		v1Client:      grpc_reflection_v1.NewServerReflectionClient(conn),
		v1alphaClient: grpc_reflection_v1alpha.NewServerReflectionClient(conn),
		receivedFiles: make(map[string]protoreflect.FileDescriptor),
	}

	return client
}

type reflectionStream interface {
	listServices() ([]string, error)
	fileContainingSymbol(fullName string) ([][]byte, error)
}

type v1ReflectionStream struct {
	client grpc_reflection_v1.ServerReflection_ServerReflectionInfoClient
}

type v1alphaReflectionStream struct {
	client grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoClient
}

// fileContainingSymbol implements reflectionStream.
func (v v1ReflectionStream) fileContainingSymbol(fullName string) ([][]byte, error) {
	err := v.client.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: fullName,
		},
	})
	if err != nil {
		return nil, err
	}
	response, err := v.client.Recv()
	if err != nil {
		return nil, err
	}
	r, ok := response.MessageResponse.(*grpc_reflection_v1.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, fmt.Errorf("excepted file descriptor response")
	}
	encodedDescriptors := r.FileDescriptorResponse.GetFileDescriptorProto()
	return encodedDescriptors, nil
}

// listServices implements reflectionStream.
func (v v1ReflectionStream) listServices() ([]string, error) {
	err := v.client.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{ListServices: ""},
	})
	if err != nil {
		return nil, err
	}
	response, err := v.client.Recv()
	if err != nil {
		return nil, err
	}
	r, ok := response.MessageResponse.(*grpc_reflection_v1.ServerReflectionResponse_ListServicesResponse)
	if !ok {
		return nil, fmt.Errorf("excepted list services response")
	}
	services := r.ListServicesResponse.GetService()
	serviceNames := make([]string, len(services))
	for i, service := range services {
		serviceNames[i] = service.GetName()
	}
	return serviceNames, nil
}
