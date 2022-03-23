package model

type ObjectsPutRequest struct {
	ContainerID string `json:"containerId"`
	FileName    string `json:"fileName"`
	Payload     string `json:"payload"`
}

type ObjectsPutResponse struct {
	ContainerID string `json:"containerId"`
	ObjectID    string `json:"objectId"`
}
