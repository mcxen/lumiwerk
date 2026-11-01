package main

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"
)

// shareService is one macOS sharing service, retained until the next menu.
type shareService struct {
	Title string
	raw   uintptr
}

var (
	objcLoad sync.Once
	objcErr  error

	objcGetClass    func(name string) uintptr
	selRegisterName func(name string) uintptr
	objcMsgSend     uintptr
	objcPoolPush    uintptr
	objcPoolPop     uintptr
	classSharing    uintptr
	classString     uintptr
	classURL        uintptr
	classArray      uintptr
	selUTF8         uintptr
	selFileURL      uintptr
	selServices     uintptr
	selCount        uintptr
	selObject       uintptr
	selTitle        uintptr
	selRetain       uintptr
	selRelease      uintptr
	selPerform      uintptr
	selStringUTF8   uintptr
	selWithObjects  uintptr
	selArray        uintptr
	selGetResource  uintptr
	selSetResource  uintptr
	selSharedSpace  uintptr
	selFileLabels   uintptr
	selAppsForURL   uintptr
	selURLPath      uintptr
	selLastComp     uintptr
	classWorkspace  uintptr
)

func loadObjC() error {
	objcLoad.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			objcErr = err
			return
		}
		purego.RegisterLibFunc(&objcGetClass, lib, "objc_getClass")
		purego.RegisterLibFunc(&selRegisterName, lib, "sel_registerName")
		objcMsgSend, objcErr = purego.Dlsym(lib, "objc_msgSend")
		if objcErr != nil {
			return
		}
		objcPoolPush, objcErr = purego.Dlsym(lib, "objc_autoreleasePoolPush")
		if objcErr != nil {
			return
		}
		objcPoolPop, objcErr = purego.Dlsym(lib, "objc_autoreleasePoolPop")
		if objcErr != nil {
			return
		}
		classSharing = objcGetClass("NSSharingService")
		classString = objcGetClass("NSString")
		classURL = objcGetClass("NSURL")
		classArray = objcGetClass("NSArray")
		selUTF8 = selRegisterName("UTF8String")
		selFileURL = selRegisterName("fileURLWithPath:")
		selServices = selRegisterName("sharingServicesForItems:")
		selCount = selRegisterName("count")
		selObject = selRegisterName("objectAtIndex:")
		selTitle = selRegisterName("title")
		selRetain = selRegisterName("retain")
		selRelease = selRegisterName("release")
		selPerform = selRegisterName("performWithItems:")
		selStringUTF8 = selRegisterName("stringWithUTF8String:")
		selWithObjects = selRegisterName("arrayWithObjects:count:")
		selArray = selRegisterName("array")
		selGetResource = selRegisterName("getResourceValue:forKey:error:")
		selSetResource = selRegisterName("setResourceValue:forKey:error:")
		selSharedSpace = selRegisterName("sharedWorkspace")
		selFileLabels = selRegisterName("fileLabels")
		selAppsForURL = selRegisterName("URLsForApplicationsToOpenURL:")
		selURLPath = selRegisterName("path")
		selLastComp = selRegisterName("lastPathComponent")
		classWorkspace = objcGetClass("NSWorkspace")
		if classString == 0 || classURL == 0 || classArray == 0 {
			objcErr = errObjC
		}
	})
	return objcErr
}

var errObjC = errorString("objc runtime unavailable")

type errorString string

func (e errorString) Error() string { return string(e) }

func ensureAppKit() {
	if loadObjC() != nil {
		logWarnf("objc runtime unavailable: %v", objcErr)
		return
	}
	if classSharing != 0 && classWorkspace != 0 {
		return
	}
	_, _ = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if classSharing == 0 {
		classSharing = objcGetClass("NSSharingService")
	}
	if classWorkspace == 0 {
		classWorkspace = objcGetClass("NSWorkspace")
	}
}

func objcMsg(self, sel uintptr, args ...uintptr) uintptr {
	if self == 0 || sel == 0 || objcMsgSend == 0 {
		return 0
	}
	call := make([]uintptr, 0, 2+len(args))
	call = append(call, self, sel)
	call = append(call, args...)
	r, _, _ := purego.SyscallN(objcMsgSend, call...)
	return r
}

func pushPool() uintptr {
	pool, _, _ := purego.SyscallN(objcPoolPush)
	return pool
}

func popPool(pool uintptr) {
	if pool != 0 {
		purego.SyscallN(objcPoolPop, pool)
	}
}

// withAutoreleasePool runs fn inside an Objective-C autorelease pool.
//
// objc_autoreleasePoolPush and objc_autoreleasePoolPop act on the calling
// thread's pool stack. A Go goroutine may migrate between OS threads between
// the two calls, so popping a pool that another thread pushed corrupts the
// runtime and segfaults. LockOSThread pins the goroutine for the pool's
// lifetime, which is what the earlier crash in popPool came from.
func withAutoreleasePool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := pushPool()
	defer popPool(pool)
	fn()
}

func nsString(s string) uintptr {
	buf := append([]byte(s), 0)
	ptr := objcMsg(classString, selStringUTF8, uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	return ptr
}

func nsArray(values []string) uintptr {
	if len(values) == 0 {
		return objcMsg(classArray, selArray)
	}
	ids := make([]uintptr, len(values))
	for i, value := range values {
		ids[i] = nsString(value)
		if ids[i] == 0 {
			return 0
		}
	}
	arr := objcMsg(classArray, selWithObjects, uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	runtime.KeepAlive(ids)
	return arr
}

func goNSString(ns uintptr) string {
	return cstrings.NSStringToString(objc.ID(ns))
}

func urlArray(paths []string) uintptr {
	if len(paths) == 0 {
		return 0
	}
	ids := make([]uintptr, len(paths))
	for i, path := range paths {
		text := nsString(path)
		ids[i] = objcMsg(classURL, selFileURL, text)
		if ids[i] == 0 {
			return 0
		}
	}
	return objcMsg(classArray, selWithObjects, uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
}

// sharingServices lists the system share sheet actions for these files.
func sharingServices(paths []string) ([]shareService, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	ensureAppKit()
	if classSharing == 0 {
		return nil, errObjC
	}
	var out []shareService
	var err error
	withAutoreleasePool(func() {
		items := urlArray(paths)
		if items == 0 {
			err = errObjC
			return
		}
		list := objcMsg(classSharing, selServices, items)
		if list == 0 {
			return
		}
		n := int(objcMsg(list, selCount))
		out = make([]shareService, 0, n)
		for i := 0; i < n; i++ {
			svc := objcMsg(list, selObject, uintptr(i))
			if svc == 0 {
				continue
			}
			title := goNSString(objcMsg(svc, selTitle))
			if title == "" {
				continue
			}
			objcMsg(svc, selRetain)
			out = append(out, shareService{Title: title, raw: svc})
		}
	})
	return out, err
}

func (s shareService) perform(paths []string) {
	if s.raw == 0 || len(paths) == 0 || loadObjC() != nil {
		return
	}
	withAutoreleasePool(func() {
		items := urlArray(paths)
		if items == 0 {
			return
		}
		objcMsg(s.raw, selRetain)
		objcMsg(s.raw, selPerform, items)
		objcMsg(s.raw, selRelease)
	})
}

func releaseShareServices(list []shareService) {
	if len(list) == 0 || loadObjC() != nil {
		return
	}
	for _, svc := range list {
		if svc.raw != 0 {
			objcMsg(svc.raw, selRelease)
		}
	}
}
