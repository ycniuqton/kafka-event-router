package diff

// Label names the words that introduce values in diff lines.
type Label string

const (
	LabelConnection    Label = "connection"
	LabelTopic         Label = "topic"
	LabelTopicPattern  Label = "topic pattern"
	LabelStartFrom     Label = "start from"
	LabelFormat        Label = "format"
	LabelKeyFormat     Label = "key format"
	LabelConsumerGroup Label = "consumer group"
	LabelKey           Label = "key"
	LabelPartitioning  Label = "partitioning"
	LabelHeaders       Label = "headers"
	LabelCreateTopic   Label = "create topic with partitions"
	LabelOnError       Label = "on error"
	LabelDLQTopic      Label = "dead-letter topic"
	LabelDLQConnection Label = "dead-letter connection"
	LabelMaxRetries    Label = "max retries"
	LabelDelivery      Label = "delivery"
	LabelName          Label = "name"
	LabelMatchAll      Label = "match all of"
	LabelMatchAny      Label = "match any of"
	LabelNot           Label = "not"
	LabelStep          Label = "step"
	LabelDisabled      Label = "(disabled)"
	LabelLimit         Label = "limit"
)

// KeyPart names fixed parts of entry keys.
type KeyPart string

const (
	KeySep       KeyPart = ":"
	KeyBranch    KeyPart = "branch"
	KeyTopic     KeyPart = "topic"
	KeyTopics    KeyPart = "topics"
	KeyStep      KeyPart = "step"
	KeyLimit     KeyPart = "limit"
	KeyFilterTag KeyPart = "filter"
)
