package tests

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"bitbucket.org/stack-rox/apollo/generated/api/v1"
	"bitbucket.org/stack-rox/apollo/pkg/clientconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	alpineDeploymentName = `alpine`
	alpineImageSha       = `7df6db5aa61ae9480f52f0b3a06a140ab98d427f86d8d5de0bedab9b8df6b1c0`
)

var (
	integration = &v1.ImageIntegration{
		Name: "public dockerhub",
		Type: "docker",
		Config: map[string]string{
			"endpoint": "registry-1.docker.io",
		},
		Categories: []v1.ImageIntegrationCategory{v1.ImageIntegrationCategory_REGISTRY},
	}
)

func TestImageIntegration(t *testing.T) {
	defer teardownAlpineDeployment(t)
	setupAlpineDeployment(t)

	conn, err := clientconn.UnauthenticatedGRPCConnection(apiEndpoint)
	require.NoError(t, err)

	subtests := []struct {
		name string
		test func(t *testing.T, conn *grpc.ClientConn)
	}{
		{
			name: "no metadata",
			test: verifyNoMetadata,
		},
		{
			name: "create",
			test: verifyCreateImageIntegration,
		},
		{
			name: "read",
			test: verifyReadImageIntegration,
		},
		{
			name: "update",
			test: verifyUpdateImageIntegration,
		},
		{
			name: "delete",
			test: verifyDeleteImageIntegration,
		},
		{
			name: "metadata populated",
			test: verifyMetadataPopulated,
		},
	}

	for _, sub := range subtests {
		t.Run(sub.name, func(t *testing.T) {
			sub.test(t, conn)
		})
	}
}

func setupAlpineDeployment(t *testing.T) {
	cmd := exec.Command(`kubectl`, `run`, alpineDeploymentName, `--image=alpine:3.7@sha256:`+alpineImageSha, `--port=22`, `--command=true`, `--`, `sleep`, `1000`)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	waitForDeployment(t, alpineDeploymentName)
}

func teardownAlpineDeployment(t *testing.T) {
	cmd := exec.Command(`kubectl`, `delete`, `deployment`, alpineDeploymentName, `--ignore-not-found=true`)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	waitForTermination(t, alpineDeploymentName)
}

func verifyNoMetadata(t *testing.T, conn *grpc.ClientConn) {
	if assertion := verifyMetadata(t, conn, func(metadata *v1.ImageMetadata) bool { return metadata == nil }); !assertion {
		t.Error("image metadata is not nil")
	}
}

func verifyMetadataPopulated(t *testing.T, conn *grpc.ClientConn) {
	t.Skip("Skipping metadata populated - AP-391")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if verifyMetadata(t, conn, func(metadata *v1.ImageMetadata) bool { return metadata != nil }) {
				return
			}
		case <-timer.C:
			t.Error("image metadata not populated after 1 minute")
			return
		}
	}
}

func verifyMetadata(t *testing.T, conn *grpc.ClientConn, assertFunc func(*v1.ImageMetadata) bool) bool {
	if assertion := verifyImageMetadata(t, conn, assertFunc); !assertion {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	deploymentService := v1.NewDeploymentServiceClient(conn)
	deployments, err := deploymentService.GetDeployments(ctx, &v1.RawQuery{
		Query: getDeploymentQuery(alpineDeploymentName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, deployments.GetDeployments())

	for _, d := range deployments.GetDeployments() {
		require.NotEmpty(t, d.GetContainers())
		c := d.GetContainers()[0]

		if assertion := assertFunc(c.GetImage().GetMetadata()); !assertion {
			return false
		}
	}

	alertService := v1.NewAlertServiceClient(conn)

	alerts, err := alertService.GetAlerts(ctx, &v1.GetAlertsRequest{
		Query: getPolicyQuery(expectedPort22Policy) + "+" + getDeploymentQuery(alpineDeploymentName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, alerts.GetAlerts())

	for _, a := range alerts.GetAlerts() {
		require.NotEmpty(t, a.GetDeployment().GetContainers())
		c := a.GetDeployment().GetContainers()[0]

		if assertion := assertFunc(c.GetImage().GetMetadata()); !assertion {
			return false
		}
	}

	return true
}

func verifyImageMetadata(t *testing.T, conn *grpc.ClientConn, assertFunc func(*v1.ImageMetadata) bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	imageService := v1.NewImageServiceClient(conn)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		image, err := imageService.GetImage(ctx, &v1.ResourceByID{Id: alpineImageSha})
		if err != nil && ctx.Err() == context.DeadlineExceeded {
			t.Error(err)
			return false
		}

		if err == nil && image != nil {
			return assertFunc(image.GetMetadata())
		}
	}

	return false
}

func verifyCreateImageIntegration(t *testing.T, conn *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	service := v1.NewImageIntegrationServiceClient(conn)

	postResp, err := service.PostImageIntegration(ctx, integration)
	require.NoError(t, err)

	integration.Id = postResp.GetId()
	assert.Equal(t, integration, postResp)
}

func verifyReadImageIntegration(t *testing.T, conn *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	service := v1.NewImageIntegrationServiceClient(conn)

	getResp, err := service.GetImageIntegration(ctx, &v1.ResourceByID{Id: integration.GetId()})
	require.NoError(t, err)
	assert.Equal(t, integration, getResp)

	getManyResp, err := service.GetImageIntegrations(ctx, &v1.GetImageIntegrationsRequest{Name: integration.GetName()})
	require.NoError(t, err)
	assert.Equal(t, 1, len(getManyResp.GetIntegrations()))
	if len(getManyResp.GetIntegrations()) > 0 {
		assert.Equal(t, integration, getManyResp.GetIntegrations()[0])
	}
}

func verifyUpdateImageIntegration(t *testing.T, conn *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	service := v1.NewImageIntegrationServiceClient(conn)

	integration.Name = "updated docker registry"

	_, err := service.PutImageIntegration(ctx, integration)
	require.NoError(t, err)

	getResp, err := service.GetImageIntegration(ctx, &v1.ResourceByID{Id: integration.GetId()})
	require.NoError(t, err)
	assert.Equal(t, integration, getResp)
}

func verifyDeleteImageIntegration(t *testing.T, conn *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	service := v1.NewImageIntegrationServiceClient(conn)

	_, err := service.DeleteImageIntegration(ctx, &v1.ResourceByID{Id: integration.GetId()})
	require.NoError(t, err)

	_, err = service.GetImageIntegration(ctx, &v1.ResourceByID{Id: integration.GetId()})
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, s.Code())
}
