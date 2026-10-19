package domain

type company struct {
    ID        string  `json:"id"`
    Name      string  `json:"name"`
    Country   string  `json:"country"`
	Valuation float64 `json:"valuation"`
}

func NewCompany(id string, name string, country string, valuation float64) *company {
	return &company{
		ID:        id,
		Name:      name,
		Country:   country,
		Valuation: valuation,
	}
}