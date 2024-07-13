package reflection

import (
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

var _ grpc_reflection_v1alpha.ServerReflectionServer = &AlphaConverter{}

type AlphaConverter struct {
	Inner grpc_reflection_v1.ServerReflectionServer
}

// ServerReflectionInfo implements grpc_reflection_v1alpha.ServerReflectionServer.
func (a AlphaConverter) ServerReflectionInfo(stream grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoServer) error {
	return a.Inner.ServerReflectionInfo(infoServer{stream})
}

type infoServer struct {
	grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoServer
}

// Recv implements grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoServer.
func (i infoServer) Recv() (*grpc_reflection_v1.ServerReflectionRequest, error) {
	req, err := i.ServerReflection_ServerReflectionInfoServer.Recv()
	if err != nil {
		return nil, err
	}
	return convertRequest(req), nil
}

// Send implements grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoServer.
func (i infoServer) Send(res *grpc_reflection_v1.ServerReflectionResponse) error {
	return i.ServerReflection_ServerReflectionInfoServer.Send(convertResponse(res))
}

func convertRequest(src *grpc_reflection_v1alpha.ServerReflectionRequest) *grpc_reflection_v1.ServerReflectionRequest {
	dest := grpc_reflection_v1.ServerReflectionRequest{
		Host: src.Host,
	}

	switch mr := src.MessageRequest.(type) {
	case *grpc_reflection_v1alpha.ServerReflectionRequest_FileByFilename:
		if mr != nil {
			dest.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_FileByFilename{
				FileByFilename: mr.FileByFilename,
			}
		}
	case *grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingSymbol:
		if mr != nil {
			dest.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: mr.FileContainingSymbol,
			}
		}
	case *grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingExtension:
		if mr != nil {
			dest.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_FileContainingExtension{
				FileContainingExtension: &grpc_reflection_v1.ExtensionRequest{
					ContainingType:  mr.FileContainingExtension.GetContainingType(),
					ExtensionNumber: mr.FileContainingExtension.GetExtensionNumber(),
				},
			}
		}
	case *grpc_reflection_v1alpha.ServerReflectionRequest_AllExtensionNumbersOfType:
		if mr != nil {
			dest.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_AllExtensionNumbersOfType{
				AllExtensionNumbersOfType: mr.AllExtensionNumbersOfType,
			}
		}
	case *grpc_reflection_v1alpha.ServerReflectionRequest_ListServices:
		if mr != nil {
			dest.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_ListServices{
				ListServices: mr.ListServices,
			}
		}
	}

	return &dest
}

func convertResponse(src *grpc_reflection_v1.ServerReflectionResponse) *grpc_reflection_v1alpha.ServerReflectionResponse {
	dest := grpc_reflection_v1alpha.ServerReflectionResponse{
		ValidHost:       src.ValidHost,
		OriginalRequest: convertOriginalRequest(src.OriginalRequest),
	}

	switch mr := src.MessageResponse.(type) {
	case *grpc_reflection_v1.ServerReflectionResponse_FileDescriptorResponse:
		if mr != nil {
			dest.MessageResponse = &grpc_reflection_v1alpha.ServerReflectionResponse_FileDescriptorResponse{
				FileDescriptorResponse: &grpc_reflection_v1alpha.FileDescriptorResponse{
					FileDescriptorProto: mr.FileDescriptorResponse.GetFileDescriptorProto(),
				},
			}
		}
	case *grpc_reflection_v1.ServerReflectionResponse_AllExtensionNumbersResponse:
		if mr != nil {
			dest.MessageResponse = &grpc_reflection_v1alpha.ServerReflectionResponse_AllExtensionNumbersResponse{
				AllExtensionNumbersResponse: &grpc_reflection_v1alpha.ExtensionNumberResponse{
					BaseTypeName:    mr.AllExtensionNumbersResponse.GetBaseTypeName(),
					ExtensionNumber: mr.AllExtensionNumbersResponse.GetExtensionNumber(),
				},
			}
		}
	case *grpc_reflection_v1.ServerReflectionResponse_ListServicesResponse:
		if mr != nil {
			services := make([]*grpc_reflection_v1alpha.ServiceResponse, len(mr.ListServicesResponse.GetService()))
			for i, svc := range mr.ListServicesResponse.GetService() {
				services[i] = &grpc_reflection_v1alpha.ServiceResponse{
					Name: svc.GetName(),
				}
			}
			dest.MessageResponse = &grpc_reflection_v1alpha.ServerReflectionResponse_ListServicesResponse{
				ListServicesResponse: &grpc_reflection_v1alpha.ListServiceResponse{
					Service: services,
				},
			}
		}
	case *grpc_reflection_v1.ServerReflectionResponse_ErrorResponse:
		if mr != nil {
			dest.MessageResponse = &grpc_reflection_v1alpha.ServerReflectionResponse_ErrorResponse{
				ErrorResponse: &grpc_reflection_v1alpha.ErrorResponse{
					ErrorCode:    mr.ErrorResponse.GetErrorCode(),
					ErrorMessage: mr.ErrorResponse.GetErrorMessage(),
				},
			}
		}
	}

	return &dest
}

func convertOriginalRequest(src *grpc_reflection_v1.ServerReflectionRequest) *grpc_reflection_v1alpha.ServerReflectionRequest {
	if src == nil {
		return nil
	}

	dest := grpc_reflection_v1alpha.ServerReflectionRequest{
		Host: src.Host,
	}

	switch mr := src.MessageRequest.(type) {
	case *grpc_reflection_v1.ServerReflectionRequest_FileByFilename:
		dest.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_FileByFilename{
			FileByFilename: mr.FileByFilename,
		}
	case *grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol:
		dest.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: mr.FileContainingSymbol,
		}
	case *grpc_reflection_v1.ServerReflectionRequest_FileContainingExtension:
		if mr.FileContainingExtension != nil {
			dest.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingExtension{
				FileContainingExtension: &grpc_reflection_v1alpha.ExtensionRequest{
					ContainingType:  mr.FileContainingExtension.GetContainingType(),
					ExtensionNumber: mr.FileContainingExtension.GetExtensionNumber(),
				},
			}
		}
	case *grpc_reflection_v1.ServerReflectionRequest_AllExtensionNumbersOfType:
		dest.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_AllExtensionNumbersOfType{
			AllExtensionNumbersOfType: mr.AllExtensionNumbersOfType,
		}
	case *grpc_reflection_v1.ServerReflectionRequest_ListServices:
		dest.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
			ListServices: mr.ListServices,
		}
	}

	return &dest
}
