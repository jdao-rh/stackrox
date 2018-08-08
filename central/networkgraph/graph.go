package networkgraph

import (
	"fmt"
	"sort"
	"sync/atomic"

	deploymentsDatastore "bitbucket.org/stack-rox/apollo/central/deployment/datastore"
	networkPolicyStore "bitbucket.org/stack-rox/apollo/central/networkpolicies/store"
	"bitbucket.org/stack-rox/apollo/generated/api/v1"
	"bitbucket.org/stack-rox/apollo/pkg/logging"
	"bitbucket.org/stack-rox/apollo/pkg/search"
	"bitbucket.org/stack-rox/apollo/pkg/set"
	"github.com/deckarep/golang-set"
	"github.com/gogo/protobuf/proto"
)

var logger = logging.LoggerForModule()

// Evaluator implements the interface for the network graph generator
type Evaluator interface {
	GetGraph(cluster *v1.Cluster, query *v1.ParsedSearchRequest) (*v1.GetNetworkGraphResponse, error)
	IncrementEpoch()
	Epoch() uint32
}

// graphEvaluatorImpl handles all of the graph calculations
type graphEvaluatorImpl struct {
	epoch uint32

	deploymentsStore   deploymentsDatastore.DataStore
	namespaceStore     ns
	networkPolicyStore networkPolicyStore.Store
}

type ns interface {
	GetNamespaces() ([]*v1.Namespace, error)
}

// newGraphEvaluator takes in namespaces
func newGraphEvaluator(deploymentsStore deploymentsDatastore.DataStore,
	namespaceStore ns, networkPolicyStore networkPolicyStore.Store) *graphEvaluatorImpl {
	return &graphEvaluatorImpl{
		deploymentsStore:   deploymentsStore,
		namespaceStore:     namespaceStore,
		networkPolicyStore: networkPolicyStore,
	}
}

func (g *graphEvaluatorImpl) IncrementEpoch() {
	atomic.AddUint32(&g.epoch, 1)
}

func (g *graphEvaluatorImpl) Epoch() uint32 {
	return atomic.LoadUint32(&g.epoch)
}

func (g *graphEvaluatorImpl) GetGraph(cluster *v1.Cluster, query *v1.ParsedSearchRequest) (*v1.GetNetworkGraphResponse, error) {
	epoch := g.Epoch()
	nodes, edges, err := g.evaluate(cluster, query)
	if err != nil {
		return nil, err
	}
	return &v1.GetNetworkGraphResponse{
		Epoch: epoch,
		Nodes: nodes,
		Edges: edges,
	}, nil
}

func (g *graphEvaluatorImpl) evaluate(cluster *v1.Cluster, query *v1.ParsedSearchRequest) (nodes []*v1.NetworkNode, edges []*v1.NetworkEdge, err error) {
	if cluster.GetId() == "" {
		return nil, nil, fmt.Errorf("cluster id must be present, but it isn't: %s", proto.MarshalTextString(cluster))
	}

	networkPolicies, err := g.networkPolicyStore.GetNetworkPolicies(&v1.GetNetworkPoliciesRequest{ClusterId: cluster.GetId()})
	if err != nil {
		return
	}

	fields := query.GetFields()
	if fields == nil {
		fields = make(map[string]*v1.ParsedSearchRequest_Values, 0)
	}
	fields[search.ClusterID] = &v1.ParsedSearchRequest_Values{Values: []string{cluster.GetId()}}
	if query == nil {
		query = &v1.ParsedSearchRequest{}
	}
	query.Fields = fields

	deployments, err := g.deploymentsStore.SearchRawDeployments(query)
	if err != nil {
		return
	}

	nodes, edges = g.evaluateCluster(deployments, networkPolicies)
	return
}

func egressNetworkPolicySelectorAppliesToDeployment(d *v1.Deployment, np *v1.NetworkPolicy) (applies bool, internetAccess bool) {
	spec := np.GetSpec()
	// Check if the src matches the pod selector and deployment then the egress rules actually apply to that deployment
	if !doesPodLabelsMatchLabel(d, spec.GetPodSelector()) || d.GetNamespace() != np.GetNamespace() {
		return
	}
	// If no egress rules are defined, then it doesn't apply
	if applies = hasEgress(spec.GetPolicyTypes()); !applies {
		return
	}

	// If there is a rule with an IPBlock that is not nil, then we can assume that they have some sort of internet access
	// This isn't exactly full proof, but probably a pretty decent indicator
	for _, rule := range spec.GetEgress() {
		for _, to := range rule.GetTo() {
			if to.IpBlock != nil {
				internetAccess = true
				return
			}
		}
	}
	return
}

func ingressNetworkPolicySelectorAppliesToDeployment(d *v1.Deployment, np *v1.NetworkPolicy) bool {
	spec := np.GetSpec()
	// Check if the src matches the pod selector and deployment then the egress rules actually apply to that deployment
	if !doesPodLabelsMatchLabel(d, spec.GetPodSelector()) || d.GetNamespace() != np.GetNamespace() {
		return false
	}
	// If no egress rules are defined, then it doesn't apply
	return hasIngress(spec.GetPolicyTypes())
}

func (g *graphEvaluatorImpl) matchPolicyPeers(d *v1.Deployment, namespace string, peers []*v1.NetworkPolicyPeer) bool {
	if len(peers) == 0 {
		return true
	}
	for _, p := range peers {
		if g.matchPolicyPeer(d, namespace, p) {
			return true
		}
	}
	return false
}

func (g *graphEvaluatorImpl) doesEgressNetworkPolicyRuleMatchDeployment(src *v1.Deployment, np *v1.NetworkPolicy) bool {
	for _, egressRule := range np.GetSpec().GetEgress() {
		if g.matchPolicyPeers(src, np.GetNamespace(), egressRule.GetTo()) {
			return true
		}
	}
	return false
}

func (g *graphEvaluatorImpl) doesIngressNetworkPolicyRuleMatchDeployment(src *v1.Deployment, np *v1.NetworkPolicy) bool {
	for _, ingressRule := range np.GetSpec().GetIngress() {
		if g.matchPolicyPeers(src, np.GetNamespace(), ingressRule.GetFrom()) {
			return true
		}
	}
	return false
}

func (g *graphEvaluatorImpl) evaluateCluster(deployments []*v1.Deployment, networkPolicies []*v1.NetworkPolicy) ([]*v1.NetworkNode, []*v1.NetworkEdge) {
	selectedDeploymentsToIngressPolicies := make(map[string]mapset.Set)
	selectedDeploymentsToEgressPolicies := make(map[string]mapset.Set)

	matchedDeploymentsToIngressPolicies := make(map[string]mapset.Set)
	matchedDeploymentsToEgressPolicies := make(map[string]mapset.Set)

	var nodes []*v1.NetworkNode
	for _, d := range deployments {
		selectedDeploymentsToIngressPolicies[d.GetId()] = mapset.NewSet()
		selectedDeploymentsToEgressPolicies[d.GetId()] = mapset.NewSet()
		matchedDeploymentsToIngressPolicies[d.GetId()] = mapset.NewSet()
		matchedDeploymentsToEgressPolicies[d.GetId()] = mapset.NewSet()

		var internetAccess bool
		for _, n := range networkPolicies {
			if n.GetSpec() == nil {
				continue
			}
			if ingressNetworkPolicySelectorAppliesToDeployment(d, n) {
				selectedDeploymentsToIngressPolicies[d.GetId()].Add(n.GetId())
			}
			if g.doesIngressNetworkPolicyRuleMatchDeployment(d, n) {
				matchedDeploymentsToIngressPolicies[d.GetId()].Add(n.GetId())
			}
			if applies, internetConnection := egressNetworkPolicySelectorAppliesToDeployment(d, n); applies {
				selectedDeploymentsToEgressPolicies[d.GetId()].Add(n.GetId())
				if internetConnection {
					internetAccess = true
				}
			}
			if g.doesEgressNetworkPolicyRuleMatchDeployment(d, n) {
				matchedDeploymentsToEgressPolicies[d.GetId()].Add(n.GetId())
			}
		}
		// If there are no egress policies, then it defaults to true
		if selectedDeploymentsToEgressPolicies[d.GetId()].Cardinality() == 0 {
			internetAccess = true
		}

		nodePoliciesSet := set.StringSliceFromSet(selectedDeploymentsToIngressPolicies[d.GetId()].Union(selectedDeploymentsToEgressPolicies[d.GetId()]))
		sort.Strings(nodePoliciesSet)

		nodes = append(nodes, &v1.NetworkNode{
			Id:             d.GetId(),
			DeploymentName: d.GetName(),
			Cluster:        d.GetClusterName(),
			Namespace:      d.GetNamespace(),
			InternetAccess: internetAccess,
			PolicyIds:      nodePoliciesSet,
		})
	}

	var edges []*v1.NetworkEdge
	for _, src := range deployments {
		for _, dst := range deployments {
			if src.GetId() == dst.GetId() {
				continue
			}

			// This set is the set of Egress policies that are applicable to the src
			selectedEgressPoliciesSet := selectedDeploymentsToEgressPolicies[src.GetId()]
			// This set is the set if Egress policies that have rules that are applicable to the dst
			matchedEgressPoliciesSet := matchedDeploymentsToEgressPolicies[dst.GetId()]
			// If there are no values in the src set of egress then it has no Egress rules and can talk to everything
			// Otherwise, if it is not empty then ensure that the intersection of the policies that apply to the source and the rules that apply to the dst have at least one in common
			if selectedEgressPoliciesSet.Cardinality() != 0 && selectedEgressPoliciesSet.Intersect(matchedEgressPoliciesSet).Cardinality() == 0 {
				continue
			}

			// This set is the set of Ingress policies that are applicable to the dst
			selectedIngressPoliciesSet := selectedDeploymentsToIngressPolicies[dst.GetId()]
			// This set is the set if Ingress policies that have rules that are applicable to the src
			matchedIngressPoliciesSet := matchedDeploymentsToIngressPolicies[src.GetId()]
			// If there are no values in the src set of egress then it has no Egress rules and can talk to everything
			// Otherwise, if it is not empty then ensure that the intersection of the policies that apply to the source and the rules that apply to the dst have at least one in common
			if selectedIngressPoliciesSet.Cardinality() != 0 && selectedIngressPoliciesSet.Intersect(matchedIngressPoliciesSet).Cardinality() == 0 {
				continue
			}

			var policyIDs []string
			policyIDs = append(policyIDs, set.StringSliceFromSet(selectedIngressPoliciesSet.Intersect(matchedIngressPoliciesSet))...)
			policyIDs = append(policyIDs, set.StringSliceFromSet(selectedEgressPoliciesSet.Intersect(matchedEgressPoliciesSet))...)

			edges = append(edges, &v1.NetworkEdge{Source: src.GetId(), Target: dst.GetId()})
		}
	}
	return nodes, edges
}

func (g *graphEvaluatorImpl) getNamespace(deployment *v1.Deployment) *v1.Namespace {
	namespaces, err := g.namespaceStore.GetNamespaces()
	if err != nil {
		return &v1.Namespace{
			Name: deployment.GetNamespace(),
		}
	}
	for _, n := range namespaces {
		if n.GetName() == deployment.GetNamespace() && n.GetClusterId() == deployment.GetClusterId() {
			return n
		}
	}
	return &v1.Namespace{
		Name: deployment.GetNamespace(),
	}
}

func (g *graphEvaluatorImpl) matchPolicyPeer(deployment *v1.Deployment, policyNamespace string, peer *v1.NetworkPolicyPeer) bool {
	if peer.IpBlock != nil {
		logger.Infof("IP Block network policy is currently not handled")
		return false
	}

	// If namespace selector is specified, then make sure the namespace matches
	// Other you fall back to the fact that the deployment must be in the policy's namespace
	if peer.GetNamespaceSelector() != nil {
		namespace := g.getNamespace(deployment)
		if !doesNamespaceMatchLabel(namespace, peer.GetNamespaceSelector()) {
			return false
		}
	} else if deployment.GetNamespace() != policyNamespace {
		return false
	}

	if peer.GetPodSelector() != nil {
		return doesPodLabelsMatchLabel(deployment, peer.GetPodSelector())
	}
	return true
}

func doesNamespaceMatchLabel(namespace *v1.Namespace, selector *v1.LabelSelector) bool {
	if len(selector.MatchLabels) == 0 {
		return true
	}
	for k, v := range namespace.GetLabels() {
		if selector.MatchLabels[k] == v {
			return true
		}
	}
	return false
}

func doesPodLabelsMatchLabel(deployment *v1.Deployment, podSelector *v1.LabelSelector) bool {
	// No values equals match all
	if len(podSelector.GetMatchLabels()) == 0 {
		return true
	}
	deploymentLabelMap := make(map[string]string)
	for _, keyValue := range deployment.GetLabels() {
		deploymentLabelMap[keyValue.GetKey()] = keyValue.GetValue()
	}
	for k, v := range podSelector.GetMatchLabels() {
		if deploymentLabelMap[k] != v {
			return false
		}
	}
	return true
}

func hasPolicyType(types []v1.NetworkPolicyType, t v1.NetworkPolicyType) bool {
	for _, pType := range types {
		if pType == t {
			return true
		}
	}
	return false
}

func hasEgress(types []v1.NetworkPolicyType) bool {
	return hasPolicyType(types, v1.NetworkPolicyType_EGRESS_NETWORK_POLICY_TYPE)
}

func hasIngress(types []v1.NetworkPolicyType) bool {
	if len(types) == 0 {
		return true
	}
	return hasPolicyType(types, v1.NetworkPolicyType_INGRESS_NETWORK_POLICY_TYPE)
}
