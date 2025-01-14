package app

import (
	"context"
	"github.com/nais/helmify/pkg/file"
	"github.com/nais/helmify/pkg/processor/job"
	"github.com/nais/helmify/pkg/processor/statefulset"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/nais/helmify/pkg/config"
	"github.com/nais/helmify/pkg/decoder"
	"github.com/nais/helmify/pkg/helm"
	"github.com/nais/helmify/pkg/processor"
	"github.com/nais/helmify/pkg/processor/configmap"
	"github.com/nais/helmify/pkg/processor/crd"
	"github.com/nais/helmify/pkg/processor/daemonset"
	"github.com/nais/helmify/pkg/processor/deployment"
	"github.com/nais/helmify/pkg/processor/rbac"
	"github.com/nais/helmify/pkg/processor/secret"
	"github.com/nais/helmify/pkg/processor/service"
	"github.com/nais/helmify/pkg/processor/storage"
	"github.com/nais/helmify/pkg/processor/webhook"
)

// Start - application entrypoint for processing input to a Helm chart.
func Start(stdin io.Reader, config config.Config) error {
	err := config.Validate()
	if err != nil {
		return err
	}
	setLogLevel(config)
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-done
		logrus.Debug("Received termination, signaling shutdown")
		cancelFunc()
	}()
	appCtx := New(config, helm.NewOutput())
	appCtx = appCtx.WithProcessors(
		configmap.New(),
		crd.New(),
		daemonset.New(),
		deployment.New(),
		statefulset.New(),
		storage.New(),
		service.New(),
		service.NewIngress(),
		rbac.ClusterRoleBinding(),
		rbac.Role(),
		rbac.RoleBinding(),
		rbac.ServiceAccount(),
		secret.New(),
		webhook.Issuer(),
		webhook.Certificate(),
		webhook.ValidatingWebhook(),
		webhook.MutatingWebhook(),
		job.NewCron(),
		job.NewJob(),
	).WithDefaultProcessor(processor.Default())
	if len(config.Files) != 0 {
		file.Walk(config.Files, config.FilesRecursively, func(filename string, fileReader io.Reader) {
			objects := decoder.Decode(ctx.Done(), fileReader)
			for obj := range objects {
				appCtx.Add(obj, filename)
			}
		})
	} else {
		objects := decoder.Decode(ctx.Done(), stdin)
		for obj := range objects {
			appCtx.Add(obj, "")
		}
	}

	return appCtx.CreateHelm(ctx.Done())
}

func setLogLevel(config config.Config) {
	logrus.SetLevel(logrus.ErrorLevel)
	if config.Verbose {
		logrus.SetLevel(logrus.InfoLevel)
	}
	if config.VeryVerbose {
		logrus.SetLevel(logrus.DebugLevel)
	}
}
