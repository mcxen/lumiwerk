package main

import (
	"os"
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
