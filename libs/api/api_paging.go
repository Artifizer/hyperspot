package api

type PageAPIRequest struct {
	PageSize   int    `query:"page_size" default:"50" maximum:"1000" doc:"Maximum number of items per page to return"`
	PageNumber int    `query:"page" default:"1" doc:"Page number"`
	Order      string `query:"order" default:"" doc:"Field to sort by, use '-' prefix for DESC ordering"`
}

func (p *PageAPIRequest) GetOffset() int {
	if p.PageNumber <= 0 {
		return 0
	}
	if p.PageSize <= 0 && p.PageNumber > 0 {
		return 0
	}
	return (p.PageNumber - 1) * p.PageSize
}

func (p *PageAPIRequest) GetStart() int {
	return p.GetOffset()
}

func (p *PageAPIRequest) GetEnd() int {
	return p.GetOffset() + p.PageSize
}

type PageAPIResponse struct {
	PageSize   int    `json:"page_size"`
	PageNumber int    `json:"page_number"`
	Order      string `json:"order"`
	Total      int    `json:"total"`
}

func PageAPIInitResponse(pageRequest *PageAPIRequest, pageResponse *PageAPIResponse) error {
	pageResponse.PageSize = pageRequest.PageSize
	pageResponse.PageNumber = pageRequest.PageNumber
	pageResponse.Order = pageRequest.Order
	return nil
}

func PageAPIPaginate[T any](items []T, pageRequest *PageAPIRequest) []T {
	if pageRequest == nil {
		return items
	}

	start := pageRequest.GetStart()
	end := pageRequest.GetEnd()
	if start > len(items) {
		return []T{}
	}
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
