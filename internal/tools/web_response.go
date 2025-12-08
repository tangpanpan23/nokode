package tools

type WebResponse struct {
	StatusCode  int               `json:"statusCode"`
	ContentType string            `json:"contentType,omitempty"`
	Body        string            `json:"body"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func CreateWebResponse(statusCode int, contentType, body string) WebResponse {
	headers := make(map[string]string)
	if contentType != "" {
		headers["Content-Type"] = contentType
	}

	if statusCode == 0 {
		statusCode = 200
	}

	return WebResponse{
		StatusCode:  statusCode,
		ContentType: contentType,
		Body:        body,
		Headers:     headers,
	}
}

