package service

import (
	"context"
	"math"
	"os"
	"path/filepath"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pkg/errors"
	"github.com/stackrox/rox/central/role"
	versionUtils "github.com/stackrox/rox/central/version/utils"
	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/pkg/env"
	"github.com/stackrox/rox/pkg/fileutils"
	"github.com/stackrox/rox/pkg/fsutils"
	"github.com/stackrox/rox/pkg/grpc/authz/user"
	"github.com/stackrox/rox/pkg/migrations"
	"github.com/stackrox/rox/pkg/postgres/pgadmin"
	"github.com/stackrox/rox/pkg/postgres/pgconfig"
	"github.com/stackrox/rox/pkg/version"
	"google.golang.org/grpc"
)

const (
	minForceRollbackTo = "3.0.58.0"
)

var (
	authorizer             = user.WithRole(role.Admin)
	capacityMarginFraction = migrations.CapacityMarginFraction + 0.05
)

type serviceImpl struct{}

// RegisterServiceServer registers this service with the given gRPC Server.
func (s *serviceImpl) RegisterServiceServer(grpcServer *grpc.Server) {
	v1.RegisterCentralHealthServiceServer(grpcServer, s)
}

// RegisterServiceHandler registers this service with the given gRPC Gateway endpoint.
func (s *serviceImpl) RegisterServiceHandler(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
	return v1.RegisterCentralHealthServiceHandler(ctx, mux, conn)
}

// AuthFuncOverride specifies the auth criteria for this API.
func (s *serviceImpl) AuthFuncOverride(ctx context.Context, fullMethodName string) (context.Context, error) {
	return ctx, authorizer.Authorized(ctx, fullMethodName)
}

// GetUpgradeStatus returns the upgrade status for Central.
func (s *serviceImpl) GetUpgradeStatus(ctx context.Context, empty *v1.Empty) (*v1.GetUpgradeStatusResponse, error) {
	if env.PostgresDatastoreEnabled.BooleanSetting() {
		// Get Postgres config data
		_, adminConfig, err := pgconfig.GetPostgresConfig()
		if err != nil {
			return nil, err
		}

		upgradeStatus := &v1.CentralUpgradeStatus{
			Version: version.GetMainVersion(),
		}
		// When using managed services, Postgres space is not a concern at this time.
		if env.ManagedCentral.BooleanSetting() {
			upgradeStatus.CanRollbackAfterUpgrade = true
		} else {
			// Check Postgres remaining capacity
			freeBytes, err := pgadmin.GetRemainingCapacity(adminConfig)
			if err != nil {
				return nil, err
			}

			currentDBBytes, err := pgadmin.GetDatabaseSize(adminConfig, migrations.GetCurrentClone())
			if err != nil {
				return nil, errors.Wrapf(err, "Fail to get database size %s", migrations.CurrentDatabase)
			}
			requiredBytes := int64(math.Ceil(float64(currentDBBytes) * (1.0 + capacityMarginFraction)))

			var toBeFreedBytes int64
			if pgadmin.CheckIfDBExists(adminConfig, migrations.PreviousDatabase) {
				toBeFreedBytes, err = pgadmin.GetDatabaseSize(adminConfig, migrations.GetPreviousClone())
				if err != nil {
					return nil, errors.Wrapf(err, "Fail to get database size %s", migrations.PreviousDatabase)
				}

				// Get a short-lived connection for the purposes of checking the version of the previous clone.
				pool := pgadmin.GetClonePool(adminConfig, migrations.GetPreviousClone())
				defer pool.Close()

				// Get rollback to version
				migVer, err := versionUtils.ReadVersionPostgres(pool)
				if err != nil {
					log.Infof("Unable to get previous version, leaving ForceRollbackTo empty.  %v", err)
				}
				if err == nil && migVer.SeqNum > 0 && version.CompareVersionsOr(migVer.MainVersion, minForceRollbackTo, -1) >= 0 {
					upgradeStatus.ForceRollbackTo = migVer.MainVersion
				}
			} else {
				// It is possible that we had a Rocks previously, so we may be able to rollback to that version.
				// Get rollback to version
				migVer, err := migrations.Read(filepath.Join(migrations.DBMountPath(), migrations.PreviousClone))
				if err != nil {
					log.Infof("Unable to get previous version, leaving ForceRollbackTo empty.  %v", err)
				}
				if err == nil && migVer.SeqNum > 0 && version.CompareVersionsOr(migVer.MainVersion, minForceRollbackTo, -1) >= 0 {
					upgradeStatus.ForceRollbackTo = migVer.MainVersion
				}
			}

			upgradeStatus.CanRollbackAfterUpgrade = freeBytes+toBeFreedBytes > requiredBytes
			upgradeStatus.SpaceAvailableForRollbackAfterUpgrade = freeBytes + toBeFreedBytes
			upgradeStatus.SpaceRequiredForRollbackAfterUpgrade = requiredBytes
		}

		return &v1.GetUpgradeStatusResponse{
			UpgradeStatus: upgradeStatus,
		}, nil
	}

	// Check persistent storage
	freeBytes, err := fsutils.AvailableBytesIn(migrations.DBMountPath())
	if err != nil {
		return nil, err
	}

	currPath, err := fileutils.ResolveIfSymlink(migrations.CurrentPath())
	if err != nil {
		return nil, err
	}
	currentDBBytes, err := fileutils.DirectorySize(currPath)
	if err != nil {
		return nil, errors.Wrapf(err, "Fail to get directory size %s", currPath)
	}
	requiredBytes := int64(math.Ceil(float64(currentDBBytes) * (1.0 + capacityMarginFraction)))

	prevPath, err := fileutils.ResolveIfSymlink(filepath.Join(migrations.DBMountPath(), migrations.GetPreviousClone()))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var toBeFreedBytes int64
	if err == nil {
		toBeFreedBytes, err = fileutils.DirectorySize(prevPath)
		if err != nil {
			return nil, errors.Wrapf(err, "Fail to get directory size %s", prevPath)
		}
	}

	upgradeStatus := &v1.CentralUpgradeStatus{
		Version:                               version.GetMainVersion(),
		CanRollbackAfterUpgrade:               int64(freeBytes)+toBeFreedBytes > requiredBytes,
		SpaceAvailableForRollbackAfterUpgrade: int64(freeBytes) + toBeFreedBytes,
		SpaceRequiredForRollbackAfterUpgrade:  requiredBytes,
	}

	// Get rollback to version
	migVer, err := migrations.Read(filepath.Join(migrations.DBMountPath(), migrations.GetPreviousClone()))
	if err != nil {
		log.Infof("Unable to get previous version, leaving ForceRollbackTo empty.  %v", err)
	}
	if err == nil && migVer.SeqNum > 0 && version.CompareVersionsOr(migVer.MainVersion, minForceRollbackTo, -1) >= 0 {
		upgradeStatus.ForceRollbackTo = migVer.MainVersion
	}

	log.Infof("Central has space to create backup: %v, currentDB: %d, free: %d, to be freed: %d with %f margin", upgradeStatus.CanRollbackAfterUpgrade, currentDBBytes, freeBytes, toBeFreedBytes, capacityMarginFraction)
	return &v1.GetUpgradeStatusResponse{
		UpgradeStatus: upgradeStatus,
	}, nil
}
