package proxy

import (
	"errors"
	"net/http"
	"sort"

	"github.com/natk64/pancake-proxy/reflection"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const (
	reflectionV1alphaService = "grpc.reflection.v1alpha.ServerReflection"
	reflectionV1Service      = "grpc.reflection.v1.ServerReflection"
)

// handleReflection check if the requested service is the reflection service and handles it, if it is.
// It returns true if the request was handled or false if the caller should handle the request.
func (p *Proxy) handleReflection(w http.ResponseWriter, r *http.Request, service string) bool {
	if service != reflectionV1Service && service != reflectionV1alphaService {
		return false
	}

	if p.disableReflectionService {
		writeGrpcStatus(w, codes.Unimplemented, "")
		return true
	}

	p.internalServer.ServeHTTP(w, r)

	return true
}

// ServerReflectionInfo implements grpc_reflection_v1.ServerReflectionServer.
func (p *Proxy) ServerReflectionInfo(stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoServer) error {
	handler := newReflectionHandler(p)

	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		getError := func(err error) *grpc_reflection_v1.ErrorResponse {
			if errors.Is(err, protoregistry.NotFound) {
				return &grpc_reflection_v1.ErrorResponse{ErrorCode: int32(codes.NotFound)}
			}

			s, ok := status.FromError(err)
			response := grpc_reflection_v1.ErrorResponse{}
			response.ErrorCode = int32(s.Code())
			if ok {
				response.ErrorMessage = s.Message()
			}

			return &response
		}

		sendError := func(err error) {
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: getError(err),
				},
			})
		}

		switch mr := msg.MessageRequest.(type) {
		case *grpc_reflection_v1.ServerReflectionRequest_ListServices:
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				OriginalRequest: msg,
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_ListServicesResponse{
					ListServicesResponse: handler.reflectionListServices(),
				},
			})

		case *grpc_reflection_v1.ServerReflectionRequest_AllExtensionNumbersOfType:
			response, err := handler.allExtensionNumbersOfType(mr.AllExtensionNumbersOfType)
			if err != nil {
				sendError(err)
				break
			}
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				OriginalRequest: msg,
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_AllExtensionNumbersResponse{
					AllExtensionNumbersResponse: response,
				},
			})
		case *grpc_reflection_v1.ServerReflectionRequest_FileByFilename:
			response, err := handler.fileByFilename(mr.FileByFilename)
			if err != nil {
				sendError(err)
				break
			}
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				OriginalRequest: msg,
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: response,
				},
			})
		case *grpc_reflection_v1.ServerReflectionRequest_FileContainingExtension:
			response, err := handler.fileContainingExtension(mr.FileContainingExtension)
			if err != nil {
				sendError(err)
				break
			}
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				OriginalRequest: msg,
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: response,
				},
			})
		case *grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol:
			response, err := handler.fileContainingSymbol(mr.FileContainingSymbol)
			if err != nil {
				sendError(err)
				break
			}
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				OriginalRequest: msg,
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: response,
				},
			})
		default:
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &grpc_reflection_v1.ErrorResponse{
						ErrorCode: int32(codes.Unimplemented),
					},
				},
			})
		}
	}
}

type reflectionHandler struct {
	sentFileDescriptors map[string]bool
	proxy               *Proxy
	resolver            reflection.FileExtensionResolver
}

func newReflectionHandler(proxy *Proxy) *reflectionHandler {
	return &reflectionHandler{
		sentFileDescriptors: make(map[string]bool),
		proxy:               proxy,
		resolver:            proxy.reflectionResolver,
	}
}

func (h *reflectionHandler) reflectionListServices() *grpc_reflection_v1.ListServiceResponse {
	h.proxy.servicesMutex.RLock()
	defer h.proxy.servicesMutex.RUnlock()

	response := &grpc_reflection_v1.ListServiceResponse{}
	for name := range h.proxy.services {
		if name == reflectionV1Service || name == reflectionV1alphaService {
			continue
		}

		response.Service = append(response.Service, &grpc_reflection_v1.ServiceResponse{
			Name: name,
		})
	}

	return response
}

func (p *reflectionHandler) fileByFilename(filename string) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	d, err := p.resolver.FindFileByPath(filename)
	if err != nil {
		return nil, err
	}

	return p.fileDescWithDependencies(d.ParentFile())
}

func (p *reflectionHandler) fileContainingSymbol(symbol string) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	d, err := p.resolver.FindDescriptorByName(protoreflect.FullName(symbol))
	if err != nil {
		return nil, err
	}

	return p.fileDescWithDependencies(d.ParentFile())
}

func (p *reflectionHandler) fileContainingExtension(ext *grpc_reflection_v1.ExtensionRequest) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	xt, err := p.resolver.FindExtensionByNumber(protoreflect.FullName(ext.ContainingType), protoreflect.FieldNumber(ext.ExtensionNumber))
	if err != nil {
		return nil, err
	}
	return p.fileDescWithDependencies(xt.ParentFile())
}

func (p *reflectionHandler) allExtensionNumbersOfType(name string) (*grpc_reflection_v1.ExtensionNumberResponse, error) {
	numbers, err := p.resolver.GetExtensionsByMessage(protoreflect.FullName(name))
	if err != nil {
		return nil, err
	}

	sort.Slice(numbers, func(i, j int) bool {
		return numbers[i] < numbers[j]
	})

	return &grpc_reflection_v1.ExtensionNumberResponse{BaseTypeName: name, ExtensionNumber: numbers}, nil
}

func (p *reflectionHandler) fileDescWithDependencies(fd protoreflect.FileDescriptor) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	if fd.IsPlaceholder() {
		return nil, protoregistry.NotFound
	}

	var r [][]byte
	queue := []protoreflect.FileDescriptor{fd}
	for len(queue) > 0 {
		currentfd := queue[0]
		queue = queue[1:]
		if currentfd.IsPlaceholder() {
			continue
		}

		if sent := p.sentFileDescriptors[currentfd.Path()]; len(r) == 0 || !sent {
			p.sentFileDescriptors[currentfd.Path()] = true
			fdProto := protodesc.ToFileDescriptorProto(currentfd)
			currentfdEncoded, err := proto.Marshal(fdProto)
			if err != nil {
				return nil, err
			}
			r = append(r, currentfdEncoded)
		}

		for i := 0; i < currentfd.Imports().Len(); i++ {
			queue = append(queue, currentfd.Imports().Get(i))
		}
	}

	return &grpc_reflection_v1.FileDescriptorResponse{FileDescriptorProto: r}, nil
}
