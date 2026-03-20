package odigos

import (
	"encoding/json"
	"time"
)

type graphQLError struct {
	Message string `json:"message"`
}

type authResponse struct {
	Data struct {
		SignIn *struct {
			IDToken     string `json:"idToken"`
			AccessToken string `json:"accessToken"`
			Status      string `json:"status"`
			User        struct {
				ID                  string            `json:"id"`
				Email               string            `json:"email"`
				Username            string            `json:"username"`
				Role                string            `json:"role"`
				NeedsPasswordChange bool              `json:"needsPasswordChange"`
				Teams               []any             `json:"teams"`
				ComputePlatforms    []ComputePlatform `json:"computePlatforms"`
				Typename            string            `json:"__typename"`
			} `json:"user"`
			Typename string `json:"__typename"`
		} `json:"signIn"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type ComputePlatform struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OdigosVersion string `json:"odigosVersion"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	ConnectedAt   int64  `json:"connectedAt"`
	LastSeenAt    int64  `json:"lastSeenAt"`
	Users         []any  `json:"users"`
	Teams         []any  `json:"teams"`
	Typename      string `json:"__typename"`
}

type computePlatformsResponse struct {
	Data struct {
		ComputePlatforms []ComputePlatform `json:"computePlatforms"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type GraphQLRemoteResponse struct {
	Data struct {
		RemoteFetch json.RawMessage `json:"remoteFetch"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type K8sActualNamespace struct {
	Name            string   `json:"name"`
	Selected        bool     `json:"selected"`
	DataStreamNames []string `json:"dataStreamNames"`
}

type namespacesResponse struct {
	Type string `json:"type"`
	Data struct {
		ComputePlatform struct {
			K8SActualNamespaces []K8sActualNamespace `json:"k8sActualNamespaces"`
		} `json:"computePlatform"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

type sourcesResponse struct {
	Type string `json:"type"`
	Data struct {
		ComputePlatform struct {
			Sources []Source `json:"sources"`
		} `json:"computePlatform"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

type sourceResponse struct {
	Type string `json:"type"`
	Data struct {
		ComputePlatform struct {
			Source Source `json:"source"`
		} `json:"computePlatform"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

// NamespaceSource is a source within a namespace (part of K8sActualNamespaceDetail).
type NamespaceSource struct {
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	Name              string   `json:"name"`
	DataStreamNames   []string `json:"dataStreamNames"`
	Selected          bool     `json:"selected"`
	NumberOfInstances int      `json:"numberOfInstances"`
}

// K8sActualNamespaceDetail is the detailed namespace payload from the API (k8sActualNamespace).
type K8sActualNamespaceDetail struct {
	Name            string           `json:"name"`
	Selected        bool             `json:"selected"`
	DataStreamNames []string         `json:"dataStreamNames"`
	Sources         []NamespaceSource `json:"sources"`
}

type namespaceResponse struct {
	Type string `json:"type"`
	Data struct {
		ComputePlatform struct {
			K8sActualNamespace K8sActualNamespaceDetail `json:"k8sActualNamespace"`
		} `json:"computePlatform"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

type SourceCondition struct {
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Message            string    `json:"message"`
	Reason             string    `json:"reason"`
	Status             string    `json:"status"`
	Type               string    `json:"type"`
}

type SourceContainer struct {
	ContainerName          string `json:"containerName"`
	InstrumentationMessage string `json:"instrumentationMessage"`
	Instrumented           bool   `json:"instrumented"`
	Language               string `json:"language"`
	OtelDistroName         string `json:"otelDistroName"`
	Overriden              bool   `json:"overriden"`
	RuntimeVersion         string `json:"runtimeVersion"`
}

type Source struct {
	Conditions      []SourceCondition `json:"conditions"`
	Containers      []SourceContainer `json:"containers"`
	DataStreamNames []string          `json:"dataStreamNames"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	OtelServiceName string            `json:"otelServiceName"`
	Selected        bool              `json:"selected"`
}

type SourceInput struct {
	CurrentStreamName string `json:"currentStreamName"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Selected          bool   `json:"selected"`
}

type SourceId struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type persistSourcesResponse struct {
	Type string `json:"type"`
	Data struct {
		PersistK8sSources bool           `json:"persistK8sSources"`
		Errors            []graphQLError `json:"errors"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

type persistNamespaceSourcesResponse struct {
	Type string `json:"type"`
	Data struct {
		PersistK8sNamespaces bool           `json:"persistK8sNamespaces"`
		Errors               []graphQLError `json:"errors"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

type NamespaceInput struct {
	CurrentStreamName string `json:"currentStreamName"`
	Namespace         string `json:"namespace"`
	Selected          bool   `json:"selected"`
}
