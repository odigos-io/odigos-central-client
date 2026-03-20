package odigos

const GetComputePlatformsQuery = `query GetComputePlatforms {
	computePlatforms {
		id
		name
		odigosVersion
		type
		status
		connectedAt
		lastSeenAt
		users {
			id
			email
			username
			role
			__typename
		}
		teams {
			id
			name
			__typename
		}
		__typename
	}
}`

const GetNamespacesQuery = `query GetNamespaces {
	computePlatform {
		k8sActualNamespaces {
			name
			selected
			dataStreamNames
		}
	}
}`

const RemoteFetchQuery = `query RemoteFetch($proxyID: String!, $query: String!, $variables: JSON!) {
	remoteFetch(proxyID: $proxyID, query: $query, variables: $variables)
}`

const GetSourcesQuery = `query GetSources {
	computePlatform {
		sources {
			namespace
			name
			kind
			dataStreamNames
			selected
			otelServiceName
			containers {
				containerName
				language
				runtimeVersion
				overriden
				instrumented
				instrumentationMessage
				otelDistroName
			}
			conditions {
				status
				type
				reason
				message
				lastTransitionTime
			}
		}
	}
}`

const GetSourceQuery = `query GetSource($sourceId: K8sSourceId!) {
	computePlatform {
		source(sourceId: $sourceId) {
			namespace
			name
			kind
			dataStreamNames
			selected
			otelServiceName
			containers {
				containerName
				language
				runtimeVersion
				overriden
				instrumented
				instrumentationMessage
				otelDistroName
			}
			conditions {
				status
				type
				reason
				message
				lastTransitionTime
			}
			manifestYAML
			instrumentationConfigYAML
		}
	}
}`

const GetNamespaceQuery = `query GetNamespace($namespaceName: String!) {
	computePlatform {
		k8sActualNamespace(name: $namespaceName) {
			name
			selected
			dataStreamNames
			sources {
				namespace
				kind
				name
				dataStreamNames
				selected
				numberOfInstances
			}
		}
	}
}`