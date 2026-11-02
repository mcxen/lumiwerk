package main

import (
	"os"
	"sync"
	"testing"
)

func TestSharingServicesForThisBinary(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if loadObjC() != nil || classSharing == 0 {
		t.Skip("AppKit is not loaded in this process")
	}
	services, err := sharingServices([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one sharing service")
	}
	for _, svc := range services {
		if svc.Title == "" || svc.raw == 0 {
			t.Fatalf("blank service: %+v", svc)
		}
	}
	releaseShareServices(services)
}

// TestSystemLabelsAndOpenersAcrossThreads hammers the Objective-C autorelease
// pool helpers from many goroutines. Go may migrate a goroutine between OS
// threads, so an unpaired pool push/pop used to corrupt the runtime and crash
// the process; pinning the thread in withAutoreleasePool keeps it safe.
func TestSystemLabelsAndOpenersAcrossThreads(t *testing.T) {
	if loadObjC() != nil {
		t.Skip("AppKit is not loaded in this process")
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		safeGo("share-stress", func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				_ = systemFinderLabels()
				_ = openerApps(path)
				if services, err := sharingServices([]string{path}); err == nil {
					releaseShareServices(services)
				}
			}
		})
	}
	wg.Wait()
}
