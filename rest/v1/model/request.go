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
