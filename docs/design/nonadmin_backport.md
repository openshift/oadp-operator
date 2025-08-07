# Non-Admin Backup and Restore Feature Backport Design
## From OADP 1.5 to OADP 1.4

### Executive Summary

This document outlines the design and implementation plan for backporting the Non-Admin Backup and Restore feature from OADP 1.5 to OADP 1.4. The feature enables non-administrative users to perform backup and restore operations in their own namespaces with controlled permissions and enforcement mechanisms.

### Current State Analysis

#### OADP 1.5 Non-Admin Feature Components

Based on analysis of the current implementation, the non-admin feature includes:

1. **New Custom Resource Definitions (CRDs)**:
   - `NonAdminBackup` - For creating backup requests
   - `NonAdminRestore` - For creating restore requests  
   - `NonAdminBackupStorageLocation` - For backup storage locations
   - `NonAdminBackupStorageLocationRequest` - For requesting BSL access
   - `NonAdminDownloadRequest` - For downloading backup logs/data

2. **Core Controller Components**:
   - Non-admin controller deployment (`internal/controller/nonadmin_controller.go`)
   - Integration with main DPA controller
   - Non-admin image management and deployment lifecycle

3. **RBAC Infrastructure**:
   - Service account for non-admin controller
   - Role and role bindings for controller permissions
   - User-facing RBAC roles (admin, editor, viewer) for each CRD

4. **DPA Integration**:
   - `NonAdmin` configuration struct in DPA spec
   - Enforcement specifications for backup/restore operations
   - BSL approval mechanisms
   - Garbage collection and sync period configurations

#### OADP 1.4 Current State

OADP 1.4 branch analysis shows:
- No non-admin CRDs present
- No non-admin controller implementation
- Basic DPA structure without NonAdmin field
- Standard RBAC limited to administrative operations

### Implementation Plan

#### Phase 1: API and CRD Implementation

**1.1 Add NonAdmin struct to DPA types** (`api/v1alpha1/dataprotectionapplication_types.go`):
```go
type NonAdmin struct {
    Enable                  *bool                              `json:"enable,omitempty"`
    EnforceBackupSpec      *velero.BackupSpec                `json:"enforceBackupSpec,omitempty"`
    EnforceRestoreSpec     *velero.RestoreSpec               `json:"enforceRestoreSpec,omitempty"`
    EnforceBSLSpec         *EnforceBackupStorageLocationSpec `json:"enforceBSLSpec,omitempty"`
    RequireApprovalForBSL  *bool                              `json:"requireApprovalForBSL,omitempty"`
    GarbageCollectionPeriod *metav1.Duration                  `json:"garbageCollectionPeriod,omitempty"`
    BackupSyncPeriod       *metav1.Duration                  `json:"backupSyncPeriod,omitempty"`
}
```

**1.2 Add required enforcement types**:
- `EnforceBackupStorageLocationSpec`
- `ObjectStorageLocation` 
- `StorageType`

**1.3 Create non-admin CRDs** (`config/crd/bases/`):
- `oadp.openshift.io_nonadminbackups.yaml`
- `oadp.openshift.io_nonadminrestores.yaml`
- `oadp.openshift.io_nonadminbackupstoragelocations.yaml`
- `oadp.openshift.io_nonadminbackupstoragelocationrequests.yaml`
- `oadp.openshift.io_nonadmindownloadrequests.yaml`
- Ensure proper validation and OpenAPI schemas
- Add short names and printer columns

**1.4 Update API constants**:
- Add `NonAdminControllerImageKey` to unsupported overrides
- Add NAC (Non-Admin Controller) defaults for periods

**1.5 Update kustomization files**:
- Add non-admin CRDs to `config/crd/kustomization.yaml`
- Add non-admin samples to `config/samples/kustomization.yaml`

#### Phase 2: Controller Implementation

**2.1 Non-admin controller logic** (`internal/controller/nonadmin_controller.go`):
- Copy complete controller implementation
- Deployment lifecycle management
- Environment variable handling including:
  - `LOG_LEVEL` - Pass through Velero log level to NAC
  - `LOG_FORMAT` - Pass through DPA log format to NAC
  - `WATCH_NAMESPACE` - OADP namespace for controller scope
- Resource version tracking for deployment updates
- Add corresponding test file (`internal/controller/nonadmin_controller_test.go`)

**2.2 DPA controller integration** (`internal/controller/dataprotectionapplication_controller.go`):
- Add `ReconcileNonAdminController` call to main reconcile loop
- Implement `checkNonAdminEnabled()` helper
- Add `getNonAdminImage()` functionality

**2.3 Validation logic** (`internal/controller/validator.go`):
- Add non-admin enforcement validation
- Update validation tests (`internal/controller/validator_test.go`)
- Add validation constants like `NACNonEnforceableErr`

**2.4 BSL integration** (`internal/controller/bsl.go`):
- Add non-admin BSL enforcement functions
- Update BSL tests (`internal/controller/bsl_test.go`)
- Integration with BSL approval mechanisms

**2.5 Helper functions and utilities**:
- Container image management  
- Label and annotation handling
- Error handling and event recording

#### Phase 3: RBAC Configuration

**3.1 Non-admin controller RBAC** (`config/non-admin-controller_rbac/`):
- Service account creation
- Controller role with required permissions
- Role binding configuration

**3.2 User-facing RBAC** (`config/rbac/`):
- Admin, editor, viewer roles for each CRD
- Proper resource and verb definitions
- Integration with existing kustomization

**3.3 Bundle manifests**:
- Update ClusterServiceVersion
- Add new RBAC cluster roles
- Ensure proper OLM packaging

#### Phase 4: Integration and Dependencies

**4.1 Main application setup** (`cmd/main.go`):
- No direct changes needed for non-admin controller
- Non-admin controller is managed by DPA controller
- Main controller already includes DPA reconciler

**4.2 Common utilities** (`pkg/common/common.go`):
- Add non-admin related constants if needed
- Ensure `LogLevelEnvVar` and `LogFormatEnvVar` constants are available (already present)
- BSL helper functions may need updates

**4.3 Manager configuration** (`config/manager/manager.yaml`):
- Add non-admin controller image environment variable (`RELATED_IMAGE_NON_ADMIN_CONTROLLER`)
- Ensure proper container definitions

**4.4 Default configuration** (`config/default/`):
- Update kustomization to include non-admin resources
- Ensure proper resource ordering

**4.5 Sample configurations**:
- Copy all non-admin sample YAML files to `config/samples/`
- `oadp_v1alpha1_nonadminbackup.yaml`
- `oadp_v1alpha1_nonadminrestore.yaml`
- `oadp_v1alpha1_nonadminbackupstoragelocation.yaml`
- `oadp_v1alpha1_nonadminbackupstoragelocationrequest.yaml`
- `oadp_v1alpha1_nonadmindownloadrequest.yaml`

**4.6 Makefile updates**:
- Add non-admin CRD generation targets
- Update bundle and manifest generation
- Add proper build dependencies

#### Phase 5: Testing and Validation

**5.1 Unit tests**:
- Port existing controller tests (`internal/controller/nonadmin_controller_test.go`)
- Update validation tests (`internal/controller/validator_test.go`)
- Update BSL tests (`internal/controller/bsl_test.go`)
- Minor update to suite test (`internal/controller/suite_test.go` - line 63)

**5.2 Integration tests**:
- Update E2E test helpers (`tests/e2e/lib/dpa_helpers.go`) (minor)
- End-to-end non-admin backup scenarios
- Permission enforcement validation
- BSL approval workflow testing

**5.3 Documentation updates**:
- API reference updates
- Configuration examples
- Feature flag documentation

### Key Dependencies and Considerations

#### Container Image Requirements

**Non-Admin Controller Image**:
- Repository: `quay.io/konveyor/oadp-non-admin`
- The controller is a separate component that must be built and maintained
- Image versioning must align with OADP 1.4 lifecycle

**Environment Variables**:
- `RELATED_IMAGE_NON_ADMIN_CONTROLLER` for image specification
- Fallback to hardcoded image path if not specified

#### Velero Version Compatibility

OADP 1.4 uses Velero 1.14.0, which should be compatible with non-admin CRDs that reference Velero v1 APIs. Key compatibility points:

- BackupSpec and RestoreSpec structures
- BackupStorageLocationSpec compatibility
- Volume snapshot/Datamover functionality

### Implementation Steps

#### Foundation
- [ ] API types and CRD implementation
- [ ] Core controller porting
- [ ] RBAC configuration

#### Integration
- [ ] DPA controller integration
- [ ] Build system and configuration updates
- [ ] Initial testing and validation

#### Testing and Refinement
- [ ] Comprehensive testing
- [ ] Documentation and final adjustments

### Risks and Mitigation Strategies

#### High Risk Items

**1. Container Image Availability**
- **Risk**: Non-admin controller image may not exist for OADP 1.4 timeframe
- **Mitigation**: Build and publish compatible image, coordinate with image pipeline team
- **Owner**: Platform/Release Engineering

**2. Velero API Compatibility**
- **Risk**: Subtle differences in Velero 1.14.0 vs newer versions used in 1.5
- **Mitigation**: Thorough testing of all Velero API interactions, version-specific validation
- **Owner**: Development Team

**3. Resource Version Conflicts**
- **Risk**: CRD conflicts if users have mixed OADP versions
- **Mitigation**: Proper upgrade/downgrade testing, clear documentation
- **Owner**: QE Team

#### Medium Risk Items

**4. Controller Resource Management**
- **Risk**: Non-admin controller deployment failures or resource contention
- **Mitigation**: Resource limit testing, failure scenario validation
- **Owner**: Development Team

**5. Feature Flag Compatibility**
- **Risk**: Dependencies on newer OADP features not available in 1.4
- **Mitigation**: Feature isolation, graceful degradation
- **Owner**: Development Team

#### Low Risk Items

**6. Documentation Gaps**
- **Risk**: Incomplete user/operator documentation
- **Mitigation**: Comprehensive doc review and testing
- **Owner**: Documentation Team

### Success Criteria

#### Functional Requirements
- [ ] Non-admin users can create backup requests in their namespaces
- [ ] Non-admin users can create restore requests in their namespaces
- [ ] Backup Storage Location approval workflow functions correctly
- [ ] Enforcement policies are properly applied
- [ ] RBAC controls prevent unauthorized access
- [ ] Garbage collection works as expected

#### Quality Requirements
- [ ] All existing OADP 1.4 functionality remains intact
- [ ] Performance impact is minimal
- [ ] Memory usage increase is reasonable
- [ ] No security vulnerabilities introduced
- [ ] Comprehensive test coverage

#### Operational Requirements
- [ ] Feature can be cleanly disabled
- [ ] Upgrade/downgrade paths are clear
- [ ] Monitoring and logging are adequate
- [ ] Documentation is complete and accurate

### Testing Strategy

#### Unit Testing
- Controller logic validation
- API validation and conversion
- Error handling scenarios

#### Integration Testing  
- End-to-end backup/restore workflows
- Multi-user permission scenarios
- BSL approval workflows
- Resource cleanup validation


### Rollback Plan

If critical issues are discovered:

1. **Immediate**: Disable non-admin feature via DPA configuration
2. **Short-term**: Remove non-admin controller deployment
3. **Long-term**: Remove CRDs and RBAC configurations if necessary

### Complete File Checklist

#### API and Types
- [ ] `api/v1alpha1/dataprotectionapplication_types.go` - Add NonAdmin struct and enforcement types

#### Custom Resource Definitions
- [ ] `config/crd/bases/oadp.openshift.io_nonadminbackups.yaml`
- [ ] `config/crd/bases/oadp.openshift.io_nonadminrestores.yaml`
- [ ] `config/crd/bases/oadp.openshift.io_nonadminbackupstoragelocations.yaml`
- [ ] `config/crd/bases/oadp.openshift.io_nonadminbackupstoragelocationrequests.yaml`
- [ ] `config/crd/bases/oadp.openshift.io_nonadmindownloadrequests.yaml`
- [ ] `config/crd/kustomization.yaml` - Add non-admin CRDs

#### Controllers and Logic
- [ ] `internal/controller/nonadmin_controller.go` - Main non-admin controller
- [ ] `internal/controller/nonadmin_controller_test.go` - Controller tests
- [ ] `internal/controller/dataprotectionapplication_controller.go` - DPA integration
- [ ] `internal/controller/validator.go` - Add validation logic
- [ ] `internal/controller/validator_test.go` - Update validation tests
- [ ] `internal/controller/bsl.go` - BSL enforcement integration
- [ ] `internal/controller/bsl_test.go` - BSL tests
- [ ] `internal/controller/suite_test.go` - Minor test setup change (line 63)

#### RBAC Configuration
- [ ] `config/non-admin-controller_rbac/service_account.yaml`
- [ ] `config/non-admin-controller_rbac/role.yaml` - Controller permissions
- [ ] `config/non-admin-controller_rbac/role_binding.yaml`
- [ ] `config/non-admin-controller_rbac/nonadmindownloadrequest_admin_role.yaml`
- [ ] `config/non-admin-controller_rbac/nonadmindownloadrequest_editor_role.yaml`
- [ ] `config/non-admin-controller_rbac/nonadmindownloadrequest_viewer_role.yaml`
- [ ] `config/non-admin-controller_rbac/kustomization.yaml`

#### Bundle and Manifests
- [ ] `bundle/manifests/oadp-operator.clusterserviceversion.yaml` - Update CSV
- [ ] `bundle/manifests/oadp.openshift.io_nonadminbackups.yaml`
- [ ] `bundle/manifests/oadp.openshift.io_nonadminrestores.yaml`
- [ ] `bundle/manifests/oadp.openshift.io_nonadminbackupstoragelocations.yaml`
- [ ] `bundle/manifests/oadp.openshift.io_nonadminbackupstoragelocationrequests.yaml`
- [ ] `bundle/manifests/oadp.openshift.io_nonadmindownloadrequests.yaml`
- [ ] `bundle/manifests/nonadmindownloadrequest-admin-role_rbac.authorization.k8s.io_v1_clusterrole.yaml`
- [ ] `bundle/manifests/nonadmindownloadrequest-editor-role_rbac.authorization.k8s.io_v1_clusterrole.yaml`
- [ ] `bundle/manifests/nonadmindownloadrequest-viewer-role_rbac.authorization.k8s.io_v1_clusterrole.yaml`

#### Configuration and Samples
- [ ] `config/manager/manager.yaml` - Add non-admin controller image env var
- [ ] `config/samples/kustomization.yaml` - Add non-admin samples
- [ ] `config/samples/oadp_v1alpha1_nonadminbackup.yaml`
- [ ] `config/samples/oadp_v1alpha1_nonadminrestore.yaml`
- [ ] `config/samples/oadp_v1alpha1_nonadminbackupstoragelocation.yaml`
- [ ] `config/samples/oadp_v1alpha1_nonadminbackupstoragelocationrequest.yaml`
- [ ] `config/samples/oadp_v1alpha1_nonadmindownloadrequest.yaml`

#### Utilities and Helpers
- [ ] `pkg/common/common.go` - Verify log level constants (`LogLevelEnvVar`, `LogFormatEnvVar`)
- [ ] `tests/e2e/lib/dpa_helpers.go` - E2E test helper updates

#### Main Application
- [ ] `cmd/main.go` - No changes needed (non-admin controller managed by DPA controller)
