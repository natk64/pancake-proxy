package reflection

import (
	"sync"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type ExtensionResolver interface {
	FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionDescriptor, error)
	GetExtensionsByMessage(message protoreflect.FullName) ([]int32, error)
}

type FileExtensionResolver interface {
	protodesc.Resolver
	ExtensionResolver
}

var _ FileExtensionResolver = (*SimpleResolver)(nil)

type SimpleResolver struct {
	files        map[string]protoreflect.FileDescriptor
	descriptors  map[protoreflect.FullName]protoreflect.Descriptor
	extensionMap extensionMap
	mutex        sync.RWMutex
}

type extensionMap map[protoreflect.FullName]map[protoreflect.FieldNumber]protoreflect.ExtensionDescriptor

// FindExtensionByNumber implements ExtensionResolver.
func (cr *SimpleResolver) FindExtensionByNumber(message protoreflect.FullName, field protowire.Number) (protoreflect.ExtensionDescriptor, error) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	extension, ok := cr.extensionMap[message][field]
	if !ok {
		return nil, protoregistry.NotFound
	}
	return extension, nil
}

// RangeExtensionsByMessage implements ExtensionResolver.
func (cr *SimpleResolver) GetExtensionsByMessage(message protoreflect.FullName) ([]int32, error) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()

	extensions, ok := cr.extensionMap[message]
	if !ok {
		return nil, protoregistry.NotFound
	}

	numbers := make([]int32, 0, len(extensions))
	for _, extension := range extensions {
		numbers = append(numbers, int32(extension.Number()))
	}

	return numbers, nil
}

// FindDescriptorByName implements protodesc.Resolver.
func (cr *SimpleResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()
	return cr.descriptors[name], nil
}

// FindFileByPath implements protodesc.Resolver.
func (cr *SimpleResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()
	return cr.files[path], nil
}

func (cr *SimpleResolver) Clear() {
	cr.files = nil
}

func (cr *SimpleResolver) RegisterFiles(fds []protoreflect.FileDescriptor) error {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()

	if cr.files == nil {
		cr.files = make(map[string]protoreflect.FileDescriptor)
	}
	if cr.descriptors == nil {
		cr.descriptors = make(map[protoreflect.FullName]protoreflect.Descriptor)
	}
	if cr.extensionMap == nil {
		cr.extensionMap = make(extensionMap)
	}

	for _, fd := range fds {
		cr.registerFileLocked(fd)
	}

	return nil
}

func (cr *SimpleResolver) registerFileLocked(fd protoreflect.FileDescriptor) {
	cr.files[fd.Path()] = fd
	services := fd.Services()
	enums := fd.Enums()
	messages := fd.Messages()
	extensions := fd.Extensions()
	imports := fd.Imports()

	for i := 0; i < services.Len(); i++ {
		service := services.Get(i)
		cr.descriptors[service.FullName()] = service
	}

	for i := 0; i < enums.Len(); i++ {
		enum := enums.Get(i)
		cr.descriptors[enum.FullName()] = enum
	}

	for i := 0; i < messages.Len(); i++ {
		msg := messages.Get(i)
		name := msg.FullName()
		cr.descriptors[name] = msg
		if cr.extensionMap[name] == nil {
			cr.extensionMap[name] = nil
		}
	}

	for i := 0; i < extensions.Len(); i++ {
		extension := extensions.Get(i)
		name := extension.FullName()
		cr.descriptors[name] = extension
		mapEntry := cr.extensionMap[name]
		if mapEntry == nil {
			mapEntry = make(map[protowire.Number]protoreflect.FieldDescriptor)
			cr.extensionMap[extension.Message().FullName()] = mapEntry
		}

		mapEntry[extension.Number()] = extension
	}

	for i := 0; i < imports.Len(); i++ {
		imported := imports.Get(i)
		if imported.IsPlaceholder() {
			continue
		}

		cr.registerFileLocked(imported)
	}
}
