package utils

const (
	UserAttributeHeaderPrefix = "X-Attribute-"
	SystemAttributePrefix     = "__NEOFS__"

	ExpirationDurationAttr  = SystemAttributePrefix + "EXPIRATION_DURATION"
	ExpirationTimestampAttr = SystemAttributePrefix + "EXPIRATION_TIMESTAMP"
	ExpirationRFC3339Attr   = SystemAttributePrefix + "EXPIRATION_RFC3339"
)
