//go:build windows && (amd64 || arm64)

package eventnative

import (
	"encoding/binary"
	"errors"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"golang.org/x/sys/windows"
)

const (
	evtRenderContextValues = uintptr(0)
	evtRenderEventValues   = uintptr(0)
	evtRenderBookmark      = uintptr(2)

	evtVariantNull     = uint32(0)
	evtVariantByte     = uint32(4)
	evtVariantUInt16   = uint32(6)
	evtVariantUInt64   = uint32(10)
	evtVariantGUID     = uint32(15)
	evtVariantFileTime = uint32(17)
	evtVariantArray    = uint32(128)

	errorNoMoreItems         = syscall.Errno(259)
	errorNotFound            = syscall.Errno(1168)
	errorEventChannelMissing = syscall.Errno(15007)
	errorEventQueryStale     = syscall.Errno(15011)
)

var (
	errNativePermission  = errors.New("NATIVE_PERMISSION_DENIED")
	errNativeUnavailable = errors.New("NATIVE_CHANNEL_UNAVAILABLE")
	errNativeNotFound    = errors.New("NATIVE_BOOKMARK_NOT_FOUND")
	errNativeFailed      = errors.New("NATIVE_EVENT_READ_FAILED")
)

type evtVariant struct {
	value  uintptr
	count  uint32
	typeID uint32
}

type rawGUID struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type systemAPI struct {
	queryProc               *windows.LazyProc
	createRenderContextProc *windows.LazyProc
	nextProc                *windows.LazyProc
	seekProc                *windows.LazyProc
	createBookmarkProc      *windows.LazyProc
	updateBookmarkProc      *windows.LazyProc
	renderProc              *windows.LazyProc
	closeProc               *windows.LazyProc
}

func newSystemAPI() (*systemAPI, error) {
	dll := windows.NewLazySystemDLL("wevtapi.dll")
	if err := dll.Load(); err != nil {
		return nil, errNativeUnavailable
	}
	api := &systemAPI{
		queryProc: dll.NewProc("EvtQuery"), createRenderContextProc: dll.NewProc("EvtCreateRenderContext"),
		nextProc: dll.NewProc("EvtNext"), seekProc: dll.NewProc("EvtSeek"),
		createBookmarkProc: dll.NewProc("EvtCreateBookmark"), updateBookmarkProc: dll.NewProc("EvtUpdateBookmark"),
		renderProc: dll.NewProc("EvtRender"), closeProc: dll.NewProc("EvtClose"),
	}
	for _, proc := range []*windows.LazyProc{api.queryProc, api.createRenderContextProc, api.nextProc, api.seekProc, api.createBookmarkProc, api.updateBookmarkProc, api.renderProc, api.closeProc} {
		if err := proc.Find(); err != nil {
			return nil, errNativeUnavailable
		}
	}
	return api, nil
}

func (a *systemAPI) query(channel, expression string, flags uint32) (handle, error) {
	channelPointer, err := windows.UTF16PtrFromString(channel)
	if err != nil {
		return 0, errNativeFailed
	}
	expressionPointer, err := windows.UTF16PtrFromString(expression)
	if err != nil {
		return 0, errNativeFailed
	}
	result, _, callErr := a.queryProc.Call(0, uintptr(unsafe.Pointer(channelPointer)), uintptr(unsafe.Pointer(expressionPointer)), uintptr(flags))
	runtime.KeepAlive(channelPointer)
	runtime.KeepAlive(expressionPointer)
	if result == 0 {
		return 0, classifySyscall(callErr)
	}
	return handle(result), nil
}

func (a *systemAPI) renderContext(paths []string) (handle, error) {
	if len(paths) != len(selectedPaths) {
		return 0, errNativeFailed
	}
	pointers := make([]*uint16, len(paths))
	for index, path := range paths {
		pointer, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return 0, errNativeFailed
		}
		pointers[index] = pointer
	}
	result, _, callErr := a.createRenderContextProc.Call(uintptr(len(pointers)), uintptr(unsafe.Pointer(&pointers[0])), evtRenderContextValues)
	runtime.KeepAlive(pointers)
	if result == 0 {
		return 0, classifySyscall(callErr)
	}
	return handle(result), nil
}

func (a *systemAPI) next(query handle) (handle, bool, error) {
	var event uintptr
	var returned uint32
	result, _, callErr := a.nextProc.Call(uintptr(query), 1, uintptr(unsafe.Pointer(&event)), 0, 0, uintptr(unsafe.Pointer(&returned)))
	if result == 0 {
		if errnoIs(callErr, errorNoMoreItems) {
			return 0, true, nil
		}
		return 0, false, classifySyscall(callErr)
	}
	if returned != 1 || event == 0 {
		if event != 0 {
			a.close(handle(event))
		}
		return 0, false, errNativeFailed
	}
	return handle(event), false, nil
}

func (a *systemAPI) seek(query, bookmark handle, offset int64, timeout, flags uint32) error {
	result, _, callErr := a.seekProc.Call(uintptr(query), int64Arg(offset), uintptr(bookmark), uintptr(timeout), uintptr(flags))
	if result == 0 {
		return classifySyscall(callErr)
	}
	return nil
}

func (a *systemAPI) createBookmark(xml *string) (handle, error) {
	var pointer *uint16
	if xml != nil {
		var err error
		pointer, err = windows.UTF16PtrFromString(*xml)
		if err != nil {
			return 0, errNativeFailed
		}
	}
	result, _, callErr := a.createBookmarkProc.Call(uintptr(unsafe.Pointer(pointer)))
	runtime.KeepAlive(pointer)
	if result == 0 {
		return 0, classifySyscall(callErr)
	}
	return handle(result), nil
}

func (a *systemAPI) updateBookmark(bookmark, event handle) error {
	result, _, callErr := a.updateBookmarkProc.Call(uintptr(bookmark), uintptr(event))
	if result == 0 {
		return classifySyscall(callErr)
	}
	return nil
}

func (a *systemAPI) renderValues(context, event handle, maximum uint32) (nativeValues, error) {
	buffer, properties, err := a.render(context, event, evtRenderEventValues, maximum)
	if err != nil {
		return nativeValues{}, err
	}
	if properties != uint32(len(selectedPaths)) {
		return nativeValues{}, errNativeFailed
	}
	return parseValues(buffer)
}

func (a *systemAPI) renderBookmark(bookmark handle, maximum uint32) (string, error) {
	buffer, properties, err := a.render(0, bookmark, evtRenderBookmark, maximum)
	if err != nil {
		return "", err
	}
	if properties != 0 || len(buffer)%2 != 0 || len(buffer) < 2 {
		return "", errNativeFailed
	}
	units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[0])), len(buffer)/2)
	if units[len(units)-1] != 0 {
		return "", errNativeFailed
	}
	units = units[:len(units)-1]
	for _, unit := range units {
		if unit == 0 {
			return "", errNativeFailed
		}
	}
	if !validUTF16(units) {
		return "", errNativeFailed
	}
	return string(utf16.Decode(units)), nil
}

func (a *systemAPI) render(context, fragment handle, flags uintptr, maximum uint32) ([]byte, uint32, error) {
	var needed, properties uint32
	result, _, callErr := a.renderProc.Call(uintptr(context), uintptr(fragment), flags, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&properties)))
	if result != 0 || !errnoIs(callErr, windows.ERROR_INSUFFICIENT_BUFFER) || needed == 0 {
		return nil, 0, errNativeFailed
	}
	if needed > maximum {
		return nil, 0, eventreader.ErrFieldTooLarge
	}
	buffer := make([]byte, needed)
	result, _, callErr = a.renderProc.Call(uintptr(context), uintptr(fragment), flags, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&properties)))
	runtime.KeepAlive(buffer)
	if result == 0 || needed > uint32(len(buffer)) {
		return nil, 0, classifySyscall(callErr)
	}
	return buffer[:needed], properties, nil
}

func (a *systemAPI) close(value handle) {
	if value != 0 {
		a.closeProc.Call(uintptr(value))
	}
}

func parseValues(buffer []byte) (nativeValues, error) {
	if len(buffer) < len(selectedPaths)*int(unsafe.Sizeof(evtVariant{})) {
		return nativeValues{}, errNativeFailed
	}
	variants := unsafe.Slice((*evtVariant)(unsafe.Pointer(&buffer[0])), len(selectedPaths))
	result := nativeValues{}
	result.fileTime, result.errs[0] = requiredUint64(variants[0], evtVariantFileTime)
	result.level, result.errs[1] = optionalByte(variants[1])
	result.eventID, result.errs[2] = requiredUint16(variants[2])
	result.guid, result.hasGUID, result.errs[3] = optionalGUID(variants[3], buffer)
	result.recordID, result.errs[4] = requiredUint64(variants[4], evtVariantUInt64)
	return result, nil
}

func requiredUint64(value evtVariant, expected uint32) (uint64, error) {
	if value.typeID == evtVariantNull {
		return 0, eventreader.ErrFieldMissing
	}
	if value.typeID != expected || value.typeID&evtVariantArray != 0 {
		return 0, eventreader.ErrFieldInvalid
	}
	return uint64(value.value), nil
}

func optionalByte(value evtVariant) (uint8, error) {
	if value.typeID == evtVariantNull {
		return 0, eventreader.ErrFieldMissing
	}
	if value.typeID != evtVariantByte || value.value > 255 {
		return 0, eventreader.ErrFieldInvalid
	}
	return uint8(value.value), nil
}

func requiredUint16(value evtVariant) (uint16, error) {
	if value.typeID == evtVariantNull {
		return 0, eventreader.ErrFieldMissing
	}
	if value.typeID != evtVariantUInt16 || value.value > 65535 {
		return 0, eventreader.ErrFieldInvalid
	}
	return uint16(value.value), nil
}

func optionalGUID(value evtVariant, buffer []byte) ([16]byte, bool, error) {
	if value.typeID == evtVariantNull {
		return [16]byte{}, false, nil
	}
	if value.typeID != evtVariantGUID || value.value == 0 {
		return [16]byte{}, false, eventreader.ErrFieldInvalid
	}
	start := uintptr(unsafe.Pointer(&buffer[0]))
	end := start + uintptr(len(buffer))
	if uintptr(value.value) < start || uintptr(value.value) > end-unsafe.Sizeof(rawGUID{}) {
		return [16]byte{}, false, eventreader.ErrFieldInvalid
	}
	offset := int(value.value - start)
	valueGUID := buffer[offset : offset+int(unsafe.Sizeof(rawGUID{}))]
	var canonical [16]byte
	binary.BigEndian.PutUint32(canonical[0:4], binary.LittleEndian.Uint32(valueGUID[0:4]))
	binary.BigEndian.PutUint16(canonical[4:6], binary.LittleEndian.Uint16(valueGUID[4:6]))
	binary.BigEndian.PutUint16(canonical[6:8], binary.LittleEndian.Uint16(valueGUID[6:8]))
	copy(canonical[8:], valueGUID[8:16])
	return canonical, true, nil
}

func classifySyscall(err error) error {
	switch {
	case errnoIs(err, windows.ERROR_ACCESS_DENIED):
		return errNativePermission
	case errnoIs(err, errorEventChannelMissing), errnoIs(err, windows.ERROR_FILE_NOT_FOUND), errnoIs(err, windows.ERROR_PATH_NOT_FOUND):
		return errNativeUnavailable
	case errnoIs(err, errorNotFound):
		return errNativeNotFound
	case errnoIs(err, errorEventQueryStale):
		return errNativeFailed
	default:
		return errNativeFailed
	}
}

func errnoIs(err error, expected syscall.Errno) bool {
	var actual syscall.Errno
	return errors.As(err, &actual) && actual == expected
}

func int64Arg(value int64) uintptr { return uintptr(value) }

func validUTF16(value []uint16) bool {
	for index := 0; index < len(value); index++ {
		current := value[index]
		switch {
		case current >= 0xd800 && current <= 0xdbff:
			index++
			if index == len(value) || value[index] < 0xdc00 || value[index] > 0xdfff {
				return false
			}
		case current >= 0xdc00 && current <= 0xdfff:
			return false
		}
	}
	return true
}

var _ eventAPI = (*systemAPI)(nil)
