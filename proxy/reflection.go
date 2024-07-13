package proxy

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

const (
	reflectionV1alphaService = "grpc.reflection.v1alpha.ServerReflection"
	reflectionV1Service      = "grpc.reflection.v1.ServerReflection"
)

// handleReflection check if the requested service is the reflection service and handles it, if it is.
// It returns true if the request was handled or false if the caller should handle the request.
func (p *proxy) handleReflection(w http.ResponseWriter, r *http.Request, service string) bool {
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
func (p *proxy) ServerReflectionInfo(stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoServer) error {
	handler := newReflectionHandler(p)

	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		sendError := func(err error) {
			s, ok := status.FromError(err)
			response := grpc_reflection_v1.ErrorResponse{}
			response.ErrorCode = int32(s.Code())
			if ok {
				response.ErrorMessage = s.Message()
			}

			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_ErrorResponse{
					ErrorResponse: &response,
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
	proxy *proxy
}

func newReflectionHandler(proxy *proxy) *reflectionHandler {
	return &reflectionHandler{proxy: proxy}
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *reflectionHandler) fileContainingSymbol(symbol string) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *reflectionHandler) fileContainingExtension(ext *grpc_reflection_v1.ExtensionRequest) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (p *reflectionHandler) allExtensionNumbersOfType(name string) (*grpc_reflection_v1.ExtensionNumberResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
