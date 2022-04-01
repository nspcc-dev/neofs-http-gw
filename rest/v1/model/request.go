package model

// ObjectsPutRequest is model for request body to upload object to NeoFS.
type ObjectsPutRequest struct {
	ContainerID string `json:"containerId"`
	FileName    string `json:"fileName"`
	Payload     string `json:"payload"`
}

// ObjectsPutResponse is model for response after upload object to NeoFS.
type ObjectsPutResponse struct {
	ContainerID string `json:"containerId"`
	ObjectID    string `json:"objectId"`
}

// ContainerTokenResponse is model for response after authentication for container operations.
type ContainerTokenResponse struct {
	Tokens []ContainerToken `json:"tokens"`
}

// ContainerToken is model for container session token.
type ContainerToken struct {
	Verb  Verb   `json:"verb"`
	Token string `json:"token"`
}

// ContainersPutRequest is model for request body to create new container in NeoFS.
type ContainersPutRequest struct {
	ContainerName   string `json:"containerName"`
	PlacementPolicy string `json:"placementPolicy"`
	BasicACL        string `json:"basicAcl"`
}

// ContainersPutResponse is model for response after put new container to NeoFS.
type ContainersPutResponse struct {
	ContainerID string `json:"containerId"`
}
