package proxy

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
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
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		switch msg.MessageRequest.(type) {
		case *grpc_reflection_v1.ServerReflectionRequest_ListServices:
			stream.Send(&grpc_reflection_v1.ServerReflectionResponse{
				OriginalRequest: msg,
				MessageResponse: &grpc_reflection_v1.ServerReflectionResponse_ListServicesResponse{
					ListServicesResponse: p.reflectionListServices(),
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

func (p *proxy) reflectionListServices() *grpc_reflection_v1.ListServiceResponse {
	p.servicesMutex.RLock()
	defer p.servicesMutex.RUnlock()

	response := &grpc_reflection_v1.ListServiceResponse{}
	for name := range p.services {
		if name == reflectionV1Service || name == reflectionV1alphaService {
			continue
		}

		response.Service = append(response.Service, &grpc_reflection_v1.ServiceResponse{
			Name: name,
		})
	}

	return response
}
