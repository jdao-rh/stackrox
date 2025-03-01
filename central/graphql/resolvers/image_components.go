package resolvers

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/graph-gophers/graphql-go"
	"github.com/pkg/errors"
	acConverter "github.com/stackrox/rox/central/activecomponent/converter"
	cveConverter "github.com/stackrox/rox/central/cve/converter/utils"
	"github.com/stackrox/rox/central/graphql/resolvers/deploymentctx"
	"github.com/stackrox/rox/central/graphql/resolvers/embeddedobjs"
	"github.com/stackrox/rox/central/graphql/resolvers/loaders"
	"github.com/stackrox/rox/central/image/mappings"
	"github.com/stackrox/rox/central/metrics"
	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/cve"
	"github.com/stackrox/rox/pkg/dackbox/edges"
	"github.com/stackrox/rox/pkg/env"
	pkgMetrics "github.com/stackrox/rox/pkg/metrics"
	"github.com/stackrox/rox/pkg/search"
	"github.com/stackrox/rox/pkg/search/scoped"
	"github.com/stackrox/rox/pkg/utils"
)

func init() {
	schema := getBuilder()
	utils.Must(
		// Resolvers for fields in storage.ImageComponent are autogenerated and located in generated.go
		// NOTE: This list is and should remain alphabetically ordered
		schema.AddExtraResolvers("ImageComponent", []string{
			"activeState(query: String): ActiveState",
			"deploymentCount(query: String, scopeQuery: String): Int!",
			"deployments(query: String, scopeQuery: String, pagination: Pagination): [Deployment!]!",
			"imageCount(query: String, scopeQuery: String): Int!",
			"images(query: String, scopeQuery: String, pagination: Pagination): [Image!]!",
			"imageVulnerabilityCount(query: String, scopeQuery: String): Int!",
			"imageVulnerabilityCounter(query: String): VulnerabilityCounter!",
			"imageVulnerabilities(query: String, scopeQuery: String, pagination: Pagination): [ImageVulnerability]!",
			"lastScanned: Time",
			"layerIndex: Int",
			"location(query: String): String!",
			"plottedImageVulnerabilities(query: String): PlottedImageVulnerabilities!",
			"topImageVulnerability: ImageVulnerability",
			"unusedVarSink(query: String): Int",

			// deprecated functions
			"fixedIn: String! @deprecated(reason: \"use 'fixedBy'\")",
		}),
		schema.AddQuery("imageComponent(id: ID): ImageComponent"),
		schema.AddQuery("imageComponents(query: String, scopeQuery: String, pagination: Pagination): [ImageComponent!]!"),
		schema.AddQuery("imageComponentCount(query: String): Int!"),
	)
}

// ImageComponentResolver represents a generic resolver of image component fields.
// Values may come from either an embedded component context, or a top level component context.
// NOTE: This list is and should remain alphabetically ordered
type ImageComponentResolver interface {
	ActiveState(ctx context.Context, args RawQuery) (*activeStateResolver, error)
	DeploymentCount(ctx context.Context, args RawQuery) (int32, error)
	Deployments(ctx context.Context, args PaginatedQuery) ([]*deploymentResolver, error)
	FixedBy(ctx context.Context) string
	Id(ctx context.Context) graphql.ID
	ImageCount(ctx context.Context, args RawQuery) (int32, error)
	Images(ctx context.Context, args PaginatedQuery) ([]*imageResolver, error)
	ImageVulnerabilityCount(ctx context.Context, args RawQuery) (int32, error)
	ImageVulnerabilityCounter(ctx context.Context, args RawQuery) (*VulnerabilityCounterResolver, error)
	ImageVulnerabilities(ctx context.Context, args PaginatedQuery) ([]ImageVulnerabilityResolver, error)
	LastScanned(ctx context.Context) (*graphql.Time, error)
	LayerIndex() (*int32, error)
	License(ctx context.Context) (*licenseResolver, error)
	Location(ctx context.Context, args RawQuery) (string, error)
	Name(ctx context.Context) string
	OperatingSystem(ctx context.Context) string
	PlottedImageVulnerabilities(ctx context.Context, args RawQuery) (*PlottedImageVulnerabilitiesResolver, error)
	Priority(ctx context.Context) int32
	RiskScore(ctx context.Context) float64
	Source(ctx context.Context) string
	TopImageVulnerability(ctx context.Context) (ImageVulnerabilityResolver, error)
	UnusedVarSink(ctx context.Context, args RawQuery) *int32
	Version(ctx context.Context) string

	// deprecated functions

	FixedIn(ctx context.Context) string
}

// ImageComponent returns an image component based on an input id (name:version)
func (resolver *Resolver) ImageComponent(ctx context.Context, args IDQuery) (ImageComponentResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ImageComponent")
	if !env.PostgresDatastoreEnabled.BooleanSetting() {
		return resolver.imageComponentV2(ctx, args)
	}

	// check permissions
	if err := readImages(ctx); err != nil {
		return nil, err
	}

	// get loader
	loader, err := loaders.GetComponentLoader(ctx)
	if err != nil {
		return nil, err
	}

	ret, err := loader.FromID(ctx, string(*args.ID))
	return resolver.wrapImageComponentWithContext(ctx, ret, true, err)
}

// ImageComponents returns image components that match the input query.
func (resolver *Resolver) ImageComponents(ctx context.Context, q PaginatedQuery) ([]ImageComponentResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ImageComponents")
	if !env.PostgresDatastoreEnabled.BooleanSetting() {
		query := queryWithImageIDRegexFilter(q.String())

		return resolver.imageComponentsV2(ctx, PaginatedQuery{Query: &query, Pagination: q.Pagination})
	}

	// check permissions
	if err := readImages(ctx); err != nil {
		return nil, err
	}

	// cast query
	query, err := q.AsV1QueryOrEmpty()
	if err != nil {
		return nil, err
	}

	// get loader
	loader, err := loaders.GetComponentLoader(ctx)
	if err != nil {
		return nil, err
	}

	// get values
	comps, err := loader.FromQuery(ctx, query)
	componentResolvers, err := resolver.wrapImageComponentsWithContext(ctx, comps, err)
	if err != nil {
		return nil, err
	}

	// cast as return type
	ret := make([]ImageComponentResolver, 0, len(componentResolvers))
	for _, res := range componentResolvers {
		ret = append(ret, res)
	}
	return ret, nil
}

// ImageComponentCount returns count of image components that match the input query
func (resolver *Resolver) ImageComponentCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ImageComponentCount")
	if !env.PostgresDatastoreEnabled.BooleanSetting() {
		query := queryWithImageIDRegexFilter(args.String())

		return resolver.componentCountV2(ctx, RawQuery{Query: &query})
	}

	// check permissions
	if err := readImages(ctx); err != nil {
		return 0, err
	}

	// cast query
	query, err := args.AsV1QueryOrEmpty()
	if err != nil {
		return 0, err
	}

	// get loader
	loader, err := loaders.GetComponentLoader(ctx)
	if err != nil {
		return 0, err
	}

	return loader.CountFromQuery(ctx, query)
}

/*
Utility Functions
*/

func (resolver *imageComponentResolver) imageComponentScopeContext() context.Context {
	if resolver.ctx == nil {
		log.Errorf("attempted to scope context on nil")
		return nil
	}
	return scoped.Context(resolver.ctx, scoped.Scope{
		Level: v1.SearchCategory_IMAGE_COMPONENTS,
		ID:    resolver.data.GetId(),
	})
}

func (resolver *imageComponentResolver) componentQuery() *v1.Query {
	return search.NewQueryBuilder().AddExactMatches(search.ComponentID, resolver.data.GetId()).ProtoQuery()
}

func (resolver *imageComponentResolver) componentRawQuery() string {
	return search.NewQueryBuilder().AddExactMatches(search.ComponentID, resolver.data.GetId()).Query()
}

func getDeploymentIDFromQuery(q *v1.Query) string {
	if q == nil {
		return ""
	}
	var deploymentID string
	search.ApplyFnToAllBaseQueries(q, func(bq *v1.BaseQuery) {
		matchFieldQuery, ok := bq.GetQuery().(*v1.BaseQuery_MatchFieldQuery)
		if !ok {
			return
		}
		if strings.EqualFold(matchFieldQuery.MatchFieldQuery.GetField(), search.DeploymentID.String()) {
			deploymentID = matchFieldQuery.MatchFieldQuery.Value
			deploymentID = strings.TrimRight(deploymentID, `"`)
			deploymentID = strings.TrimLeft(deploymentID, `"`)
		}
	})
	return deploymentID
}

func getDeploymentScope(scopeQuery *v1.Query, contexts ...context.Context) string {
	for _, ctx := range contexts {
		if scope, ok := scoped.GetScope(ctx); ok && scope.Level == v1.SearchCategory_DEPLOYMENTS {
			return scope.ID
		} else if deploymentID := deploymentctx.FromContext(ctx); deploymentID != "" {
			return deploymentID
		}
	}
	if scopeQuery != nil {
		return getDeploymentIDFromQuery(scopeQuery)
	}
	return ""
}

func getImageIDFromScope(contexts ...context.Context) string {
	for _, ctx := range contexts {
		if scope, ok := scoped.GetScope(ctx); ok {
			if scope.Level == v1.SearchCategory_IMAGES {
				return scope.ID
			}
		}
	}
	return ""
}

func queryWithImageIDRegexFilter(q string) string {
	return search.AddRawQueriesAsConjunction(q,
		search.NewQueryBuilder().AddRegexes(search.ImageSHA, ".+").Query())
}

func getImageCVEResolvers(ctx context.Context, root *Resolver, os string, vulns []*storage.EmbeddedVulnerability, query *v1.Query) ([]ImageVulnerabilityResolver, error) {
	query, _ = search.FilterQueryWithMap(query, mappings.VulnerabilityOptionsMap)
	predicate, err := vulnPredicateFactory.GeneratePredicate(query)
	if err != nil {
		return nil, err
	}

	// Use the images to map CVEs to the images and components.
	idToVals := make(map[string]*imageCVEResolver)
	for _, vuln := range vulns {
		if !predicate.Matches(vuln) {
			continue
		}
		id := cve.ID(vuln.GetCve(), os)
		if _, exists := idToVals[id]; !exists {
			converted := cveConverter.EmbeddedVulnerabilityToImageCVE(os, vuln)
			resolver, err := root.wrapImageCVE(converted, true, nil)
			if err != nil {
				return nil, err
			}
			resolver.ctx = embeddedobjs.VulnContext(ctx, vuln)
			idToVals[id] = resolver
		}
	}

	// For now, sort by ID.
	resolvers := make([]*imageCVEResolver, 0, len(idToVals))
	for _, vuln := range idToVals {
		resolvers = append(resolvers, vuln)
	}
	if len(query.GetPagination().GetSortOptions()) == 0 {
		sort.SliceStable(resolvers, func(i, j int) bool {
			return resolvers[i].data.GetId() < resolvers[j].data.GetId()
		})
	}
	resolverI := make([]ImageVulnerabilityResolver, 0, len(resolvers))
	for _, resolver := range resolvers {
		resolverI = append(resolverI, resolver)
	}
	return paginate(query.GetPagination(), resolverI, nil)
}

/*
Sub Resolver Functions
*/

func (resolver *imageComponentResolver) ActiveState(ctx context.Context, args RawQuery) (*activeStateResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "ActiveState")
	scopeQuery, err := args.AsV1QueryOrEmpty()
	if err != nil {
		return nil, err
	}

	deploymentID := getDeploymentScope(scopeQuery, resolver.ctx)
	if deploymentID == "" {
		return nil, nil
	}

	if resolver.data.GetSource() != storage.SourceType_OS {
		return &activeStateResolver{
			root:  resolver.root,
			state: Undetermined,
		}, nil
	}
	acID := acConverter.ComposeID(deploymentID, resolver.data.GetId())

	var found bool
	imageID := getImageIDFromQuery(scopeQuery)
	if imageID == "" {
		found, err = resolver.root.ActiveComponent.Exists(ctx, acID)
		if err != nil {
			return nil, err
		}
	} else {
		query := search.NewQueryBuilder().AddExactMatches(search.ImageSHA, imageID).ProtoQuery()
		results, err := resolver.root.ActiveComponent.Search(ctx, query)
		if err != nil {
			return nil, err
		}
		found = search.ResultsToIDSet(results).Contains(acID)
	}
	if !found {
		return &activeStateResolver{
			root:  resolver.root,
			state: Inactive,
		}, nil
	}

	return &activeStateResolver{
		root:               resolver.root,
		state:              Active,
		activeComponentIDs: []string{acID},
		imageScope:         imageID,
	}, nil
}

func (resolver *imageComponentResolver) DeploymentCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "DeploymentCount")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.DeploymentCount(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) Deployments(ctx context.Context, args PaginatedQuery) ([]*deploymentResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "Deployments")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.Deployments(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) ImageCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "ImageCount")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.ImageCount(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) Images(ctx context.Context, args PaginatedQuery) ([]*imageResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "Images")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.Images(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) ImageVulnerabilityCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "ImageVulnerabilityCount")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.ImageVulnerabilityCount(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) ImageVulnerabilityCounter(ctx context.Context, args RawQuery) (*VulnerabilityCounterResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "ImageVulnerabilityCounter")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.ImageVulnerabilityCounter(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) ImageVulnerabilities(ctx context.Context, args PaginatedQuery) ([]ImageVulnerabilityResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "ImageVulnerabilities")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}

	// Short path. Full image is embedded when image scan resolver is called.
	embeddedComponent := embeddedobjs.ComponentFromContext(resolver.ctx)
	if embeddedComponent == nil {
		return resolver.root.ImageVulnerabilities(resolver.imageComponentScopeContext(), args)
	}

	query, err := args.AsV1QueryOrEmpty()
	if err != nil {
		return nil, err
	}
	return getImageCVEResolvers(resolver.ctx, resolver.root, embeddedobjs.OSFromContext(resolver.ctx), embeddedComponent.GetVulns(), query)
}

func (resolver *imageComponentResolver) LastScanned(ctx context.Context) (*graphql.Time, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "LastScanned")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}

	// Short path. Full image is embedded when image scan resolver is called.
	if scanTime := embeddedobjs.LastScannedFromContext(resolver.ctx); scanTime != nil {
		return timestamp(scanTime)
	}

	imageLoader, err := loaders.GetImageLoader(resolver.ctx)
	if err != nil {
		return nil, err
	}

	q := resolver.componentQuery()
	q.Pagination = &v1.QueryPagination{
		Limit:  1,
		Offset: 0,
		SortOptions: []*v1.QuerySortOption{
			{
				Field:    search.ImageScanTime.String(),
				Reversed: true,
			},
		},
	}

	images, err := imageLoader.FromQuery(resolver.ctx, q)
	if err != nil || len(images) == 0 {
		return nil, err
	} else if len(images) > 1 {
		return nil, errors.New("multiple images matched for last scanned image component query")
	}

	return timestamp(images[0].GetScan().GetScanTime())
}

// Location returns the location of the component.
func (resolver *imageComponentResolver) Location(ctx context.Context, args RawQuery) (string, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "Location")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}

	// Short path. Full image is embedded when image scan resolver is called.
	if embeddedComponent := embeddedobjs.ComponentFromContext(resolver.ctx); embeddedComponent != nil {
		return embeddedComponent.GetLocation(), nil
	}

	imageID := getImageIDFromScope(resolver.ctx)
	if imageID == "" {
		var err error
		imageID, err = getImageIDFromIfImageShaQuery(resolver.ctx, resolver.root, args)
		if err != nil {
			return "", errors.Wrap(err, "could not determine component location")
		}
	}
	if imageID == "" {
		return "", nil
	}

	if !env.PostgresDatastoreEnabled.BooleanSetting() {
		edgeID := edges.EdgeID{ParentID: imageID, ChildID: resolver.data.GetId()}.ToString()
		edge, found, err := resolver.root.ImageComponentEdgeDataStore.Get(resolver.ctx, edgeID)
		if err != nil || !found {
			return "", err
		}
		return edge.GetLocation(), nil
	}
	query := search.NewQueryBuilder().AddExactMatches(search.ImageSHA, imageID).AddExactMatches(search.ComponentID, resolver.data.GetId()).ProtoQuery()
	edges, err := resolver.root.ImageComponentEdgeDataStore.SearchRawEdges(resolver.ctx, query)
	if err != nil || len(edges) == 0 {
		return "", err
	}
	return edges[0].GetLocation(), nil
}

// PlottedImageVulnerabilities returns the data required by top risky entity scatter-plot on vuln mgmt dashboard
func (resolver *imageComponentResolver) PlottedImageVulnerabilities(ctx context.Context, args RawQuery) (*PlottedImageVulnerabilitiesResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "PlottedImageVulnerabilities")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}
	return resolver.root.PlottedImageVulnerabilities(resolver.imageComponentScopeContext(), args)
}

func (resolver *imageComponentResolver) TopImageVulnerability(ctx context.Context) (ImageVulnerabilityResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.ImageComponents, "TopImageVulnerability")
	if resolver.ctx == nil {
		resolver.ctx = ctx
	}

	// Short path. Full image is embedded when image scan resolver is called.
	if embeddedComponent := embeddedobjs.ComponentFromContext(resolver.ctx); embeddedComponent != nil {
		var topVuln *storage.EmbeddedVulnerability
		for _, vuln := range embeddedComponent.GetVulns() {
			if topVuln == nil || vuln.GetCvss() > topVuln.GetCvss() {
				topVuln = vuln
			}
		}
		if topVuln == nil {
			return nil, nil
		}
		return resolver.root.wrapImageCVEWithContext(resolver.ctx,
			cveConverter.EmbeddedVulnerabilityToImageCVE(embeddedobjs.OSFromContext(resolver.ctx), topVuln), true, nil,
		)
	}

	if !env.PostgresDatastoreEnabled.BooleanSetting() {
		vulnResolver, err := resolver.unwrappedTopVulnQuery(resolver.ctx)
		if err != nil || vulnResolver == nil {
			return nil, err
		}
		return vulnResolver, nil
	}
	return resolver.root.TopImageVulnerability(resolver.imageComponentScopeContext(), RawQuery{})
}

func (resolver *imageComponentResolver) LayerIndex() (*int32, error) {
	// Short path. Full image is embedded when image scan resolver is called.
	if embeddedComponent := embeddedobjs.ComponentFromContext(resolver.ctx); embeddedComponent != nil {
		w, ok := embeddedComponent.GetHasLayerIndex().(*storage.EmbeddedImageScanComponent_LayerIndex)
		if !ok {
			return nil, nil
		}
		v := w.LayerIndex
		return &v, nil
	}

	scope, hasScope := scoped.GetScope(resolver.ctx)
	if !hasScope || scope.Level != v1.SearchCategory_IMAGES {
		return nil, nil
	}
	edges, err := resolver.root.ImageComponentEdgeDataStore.SearchRawEdges(resolver.ctx, resolver.componentQuery())
	if err != nil {
		return nil, err
	}
	if len(edges) == 0 || len(edges) > 1 {
		return nil, errors.Errorf("Unexpected number of image-component edge matched for image %s and component %s. Expected 1 edge.", scope.ID, resolver.data.GetId())
	}

	w, ok := edges[0].GetHasLayerIndex().(*storage.ImageComponentEdge_LayerIndex)
	if !ok {
		return nil, nil
	}
	v := w.LayerIndex
	return &v, nil
}

func (resolver *imageComponentResolver) UnusedVarSink(_ context.Context, _ RawQuery) *int32 {
	return nil
}

// Following are deprecated functions that are retained to allow UI time to migrate away from them

func (resolver *imageComponentResolver) FixedIn(_ context.Context) string {
	return resolver.data.GetFixedBy()
}
