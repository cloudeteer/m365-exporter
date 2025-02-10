package exchange

type MailFlowResponse struct {
	Value []MailFlowResponseRow `json:"value"`
}

type MailFlowResponseRow struct {
	Organization string `json:"Organization"`
	Date         string `json:"Date"`
	EventType    string `json:"EventType"`
	Direction    string `json:"Direction"`
	MessageCount int    `json:"MessageCount"`
	Index        int    `json:"Index"`
}
