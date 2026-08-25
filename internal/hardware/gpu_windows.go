//go:build windows

package hardware

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegEnumKeyExW    = advapi32.NewProc("RegEnumKeyExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

const (
	hkeyLocalMachine   = 0x80000002
	keyRead            = 0x20019
	regSz              = 1
	displayClassGUID   = `{4d36e968-e325-11ce-bfc1-08002be10318}`
	maxKeyLength       = 255
	maxValueNameLength = 16383
	maxValueDataLength = 16384
)

// detectGPU enumerates display adapters from the Windows registry
// under the display class GUID. Returns the first GPU with usable info.
func detectGPU() (GPUInfo, error) {
	var hKey syscall.Handle
	ret, _, _ := procRegOpenKeyExW.Call(
		hkeyLocalMachine,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(`SYSTEM\CurrentControlSet\Control\Class\`+displayClassGUID))),
		0,
		keyRead,
		uintptr(unsafe.Pointer(&hKey)),
	)
	if ret != 0 {
		return GPUInfo{}, fmt.Errorf("failed to open display class registry key")
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	var index uint32
	for {
		var subKeyName [maxKeyLength + 1]uint16
		subKeyNameLen := uint32(len(subKeyName))
		ret, _, _ := procRegEnumKeyExW.Call(
			uintptr(hKey),
			uintptr(index),
			uintptr(unsafe.Pointer(&subKeyName[0])),
			uintptr(unsafe.Pointer(&subKeyNameLen)),
			0, 0, 0, 0,
		)
		if ret != 0 {
			break
		}
		index++

		subKey := syscall.UTF16ToString(subKeyName[:subKeyNameLen])
		if gpu, ok := queryAdapterKey(hKey, subKey); ok {
			return gpu, nil
		}
	}

	return GPUInfo{}, fmt.Errorf("no GPU detected in display class registry")
}

func queryAdapterKey(parentKey syscall.Handle, subKey string) (GPUInfo, bool) {
	var hSubKey syscall.Handle
	subKeyPath := `SYSTEM\CurrentControlSet\Control\Class\` + displayClassGUID + `\` + subKey
	ret, _, _ := procRegOpenKeyExW.Call(
		hkeyLocalMachine,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(subKeyPath))),
		0,
		keyRead,
		uintptr(unsafe.Pointer(&hSubKey)),
	)
	if ret != 0 {
		return GPUInfo{}, false
	}
	defer procRegCloseKey.Call(uintptr(hSubKey))

	var vendor, name string
	var vramBytes uint64
	var vramKnown bool

	// Query DriverDesc for name
	if val, ok := queryRegString(hSubKey, "DriverDesc"); ok {
		name = val
	}
	// Query ProviderName for vendor
	if val, ok := queryRegString(hSubKey, "ProviderName"); ok {
		vendor = val
	}
	// Query HardwareInformation.qwMemorySize for VRAM (if present)
	if val, ok := queryRegQword(hSubKey, "HardwareInformation.qwMemorySize"); ok {
		vramBytes = val
		vramKnown = true
	}

	if name == "" && vendor == "" {
		return GPUInfo{}, false
	}

	return GPUInfo{
		Vendor:    vendor,
		Name:      name,
		VRAMBytes: vramBytes,
		VRAMKnown: vramKnown,
		Known:     true,
	}, true
}

func queryRegString(hKey syscall.Handle, valueName string) (string, bool) {
	var valType uint32
	var dataLen uint32
	valNamePtr, _ := syscall.UTF16PtrFromString(valueName)

	ret, _, _ := procRegQueryValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valNamePtr)),
		0,
		uintptr(unsafe.Pointer(&valType)),
		0,
		uintptr(unsafe.Pointer(&dataLen)),
	)
	if ret != 0 || valType != regSz || dataLen == 0 {
		return "", false
	}

	buf := make([]uint16, dataLen/2)
	ret, _, _ = procRegQueryValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valNamePtr)),
		0,
		uintptr(unsafe.Pointer(&valType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&dataLen)),
	)
	if ret != 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func queryRegQword(hKey syscall.Handle, valueName string) (uint64, bool) {
	var valType uint32
	var dataLen uint32 = 8
	valNamePtr, _ := syscall.UTF16PtrFromString(valueName)

	var value uint64
	ret, _, _ := procRegQueryValueExW.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valNamePtr)),
		0,
		uintptr(unsafe.Pointer(&valType)),
		uintptr(unsafe.Pointer(&value)),
		uintptr(unsafe.Pointer(&dataLen)),
	)
	if ret != 0 || valType != 11 { // REG_QWORD = 11
		return 0, false
	}
	return value, true
}
