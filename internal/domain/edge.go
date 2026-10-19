package domain

type edge struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	Product   string  `json:"product"`
	Weight    float32 `json:"weight"`
	Exclusive bool    `json:"exclusive"`
}

func NewEdge(from string, to string, product string, weight float32, exclusive bool) *edge {
	return &edge{
		From:      from,
		To:        to,
		Product:   product,
		Weight:    weight,
		Exclusive: exclusive,
	}
}
