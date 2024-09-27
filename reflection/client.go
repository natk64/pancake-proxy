package reflection

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type ReflectionClient struct {
	mu            sync.Mutex
	v1Client      grpc_reflection_v1.ServerReflectionClient
	v1alphaClient grpc_reflection_v1alpha.ServerReflectionClient

	v1           reflectionStream
	v1alpha      reflectionStream
	disconnected chan struct{}
}

// Connected returns true if the client has an open stream to the server.
func (client *ReflectionClient) Connected() bool {
	return client.v1 != nil || client.v1alpha != nil
}

// Disconnected returns a channel that will be closed when the underlying stream disconnects.
func (client *ReflectionClient) Disconnected() <-chan struct{} {
	if client.disconnected == nil {
		c := make(chan struct{})
		close(c)
		return c
	}

	return client.disconnected
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

	descriptorProtos := make([]*descriptorpb.FileDescriptorProto, len(encodedDescriptors))
	for i, data := range encodedDescriptors {
		var descriptor descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(data, &descriptor); err != nil {
			return nil, err
		}
		descriptorProtos[i] = &descriptor
	}

	descriptorMap, err := desc.CreateFileDescriptors(descriptorProtos)
	if err != nil {
		return nil, err
	}

	fileDescriptors := make([]protoreflect.FileDescriptor, 0, len(descriptorMap))
	for _, descriptor := range descriptorMap {
		fileDescriptors = append(fileDescriptors, descriptor.UnwrapFile())
	}

	return fileDescriptors, nil
}

func (client *ReflectionClient) getStream() (reflectionStream, error) {
	if client.v1 != nil {
		return client.v1, nil
	}
	if client.v1alpha != nil {
		return client.v1alpha, nil
	}

	stream, err := client.v1Client.ServerReflectionInfo(context.Background())
	if err == nil {
		client.v1 = newV1Stream(stream, client.streamDisconnected)
		client.disconnected = make(chan struct{})
		return client.v1, err
	}

	alphaStream, err := client.v1alphaClient.ServerReflectionInfo(context.Background())
	if err != nil {
		return nil, err
	}

	client.disconnected = make(chan struct{})
	client.v1alpha = newV1AlphaStream(alphaStream, client.streamDisconnected)
	return client.v1alpha, nil
}

func (client *ReflectionClient) streamDisconnected() {
	if client.disconnected != nil {
		close(client.disconnected)
	}
	client.v1 = nil
	client.v1alpha = nil
}

func NewClient(conn *grpc.ClientConn) *ReflectionClient {
	client := &ReflectionClient{
		v1Client:      grpc_reflection_v1.NewServerReflectionClient(conn),
		v1alphaClient: grpc_reflection_v1alpha.NewServerReflectionClient(conn),
	}

	return client
}

type v1Stream = grpc_reflection_v1.ServerReflection_ServerReflectionInfoClient
type v1alphaStream = grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoClient

type reflectionStream interface {
	listServices() ([]string, error)
	fileContainingSymbol(fullName string) ([][]byte, error)
}

func newV1Stream(stream v1Stream, onDisconnect func()) reflectionStream {
	return v1ReflectionStream{
		sender:   stream,
		receiver: newBufferedReceiver(stream, func(err error) { onDisconnect() }),
	}
}

func newV1AlphaStream(stream v1alphaStream, onDisconnect func()) reflectionStream {
	return v1alphaReflectionStream{
		sender:   stream,
		receiver: newBufferedReceiver(stream, func(err error) { onDisconnect() }),
	}
}

type v1ReflectionStream struct {
	sender   sender[*grpc_reflection_v1.ServerReflectionRequest]
	receiver receiver[*grpc_reflection_v1.ServerReflectionResponse]
}

func (v v1ReflectionStream) fileContainingSymbol(fullName string) ([][]byte, error) {
	err := v.sender.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: fullName,
		},
	})
	if err != nil {
		return nil, err
	}
	response, err := v.receiver.Recv()
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

func (v v1ReflectionStream) listServices() ([]string, error) {
	err := v.sender.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{ListServices: ""},
	})
	if err != nil {
		return nil, err
	}
	response, err := v.receiver.Recv()
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

type v1alphaReflectionStream struct {
	sender   sender[*grpc_reflection_v1alpha.ServerReflectionRequest]
	receiver receiver[*grpc_reflection_v1alpha.ServerReflectionResponse]
}

func (v v1alphaReflectionStream) fileContainingSymbol(fullName string) ([][]byte, error) {
	err := v.sender.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: fullName,
		},
	})
	if err != nil {
		return nil, err
	}
	response, err := v.receiver.Recv()
	if err != nil {
		return nil, err
	}
	r, ok := response.MessageResponse.(*grpc_reflection_v1alpha.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, fmt.Errorf("excepted file descriptor response")
	}
	encodedDescriptors := r.FileDescriptorResponse.GetFileDescriptorProto()
	return encodedDescriptors, nil
}

func (v v1alphaReflectionStream) listServices() ([]string, error) {
	err := v.sender.Send(&grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{ListServices: ""},
	})
	if err != nil {
		return nil, err
	}
	response, err := v.receiver.Recv()
	if err != nil {
		return nil, err
	}
	r, ok := response.MessageResponse.(*grpc_reflection_v1alpha.ServerReflectionResponse_ListServicesResponse)
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

type receiver[T any] interface {
	Recv() (T, error)
}

type sender[T any] interface {
	Send(req T) error
}

type resultTuple[T any] struct {
	msg T
	err error
}

type bufferedReceiver[T any] struct {
	OnError func(err error)

	c chan resultTuple[T]
}

func newBufferedReceiver[T any](receiver receiver[T], onError func(err error)) receiver[T] {
	buffer := bufferedReceiver[T]{
		c:       make(chan resultTuple[T], 1),
		OnError: onError,
	}

	go func() {
		defer close(buffer.c)

		for {
			msg, err := receiver.Recv()
			select {
			case buffer.c <- resultTuple[T]{msg: msg, err: err}:
			default:
				onError(errors.Join(err, fmt.Errorf("receive buffer full")))
				return
			}

			if err != nil {
				onError(err)
				return
			}
		}
	}()

	return buffer
}

func (buffer bufferedReceiver[T]) Recv() (t T, err error) {
	result, ok := <-buffer.c
	if !ok {
		return t, fmt.Errorf("receive buffer closed")
	}

	return result.msg, result.err
}
