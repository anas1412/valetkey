package leader

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestOnePodLeadsAndHandsOver(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{leaseGVR: "LeaseList"})
	a := &Elector{Client: client, Namespace: "ns", Name: "valetkey", Identity: "pod-a", Duration: time.Second, Renew: 50 * time.Millisecond}
	b := &Elector{Client: client, Namespace: "ns", Name: "valetkey", Identity: "pod-b", Duration: time.Second, Renew: 50 * time.Millisecond}

	ctxA, stopA := context.WithCancel(context.Background())
	led := make(chan string, 4)
	go a.Run(ctxA, func() { led <- "pod-a" })
	if got := <-led; got != "pod-a" {
		t.Fatal(got)
	}
	ctxB, stopB := context.WithCancel(context.Background())
	defer stopB()
	go b.Run(ctxB, func() { led <- "pod-b" })
	time.Sleep(300 * time.Millisecond)
	if b.Leading() || !a.Leading() {
		t.Fatalf("a=%v b=%v: only pod-a should lead", a.Leading(), b.Leading())
	}

	stopA() // pod-a shuts down and releases the Lease
	select {
	case got := <-led:
		if got != "pod-b" {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pod-b never took over")
	}
}
