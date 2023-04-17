package main

import (
	"context"
	"flag"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	klog.InitFlags(nil)
	var (
		kubeconfig         string
		leaseLockName      string
		leaseLockNamespace string
		id                 string
	)
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&id, "id", uuid.New().String(), "the holder identity name")
	flag.StringVar(&leaseLockName, "lease-lock-name", "", "the lease lock resource name")
	flag.StringVar(&leaseLockNamespace, "lease-lock-namespace", "", "the lease lock namespace")
	flag.Parse()
	if strings.EqualFold(leaseLockName, "") {
		klog.Fatalln("unable to get lease resource name (missing lease-lock-name flag).")
	}
	if strings.EqualFold(leaseLockNamespace, "") {
		klog.Fatalln("unable to get lease resource namespace (missing lease-lock-namespace flag).")
	}

	config, err := buildConfig(kubeconfig)
	if err != nil {
		klog.Fatalln(err)
	}

	client := clientset.NewForConfigOrDie(config)
	run := func(ctx context.Context) {
		klog.Info("Controller loop ...")
		select {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		klog.Info("Received termination, signaling shutdown")
		cancel()
	}()

	lock := &resourcelock.LeaseLock{
		LeaseMeta: v1.ObjectMeta{
			Name:      leaseLockName,
			Namespace: leaseLockNamespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: 60 * time.Second,
		RenewDeadline: 15 * time.Second,
		RetryPeriod:   5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				run(ctx)
			},
			OnStoppedLeading: func() {
				klog.Info("leader lost: %s", id)
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					return
				}
				klog.Infof("new leader elected: %s", identity)
			},
		},
		WatchDog:        nil,
		ReleaseOnCancel: true,
	})
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if !strings.EqualFold(kubeconfig, "") {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
