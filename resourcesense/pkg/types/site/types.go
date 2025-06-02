package site

type Site struct {
	Name        string `json:"site" bson:"site"`
	DisplayName string `json:"displayName" bson:"displayName"`
	Domain      string `json:"domain" bson:"domain"`
	AdminDomain string `json:"adminDomain" bson:"adminDomain"`
	IsCenter    bool   `json:"isCenter" bson:"isCenter"`
	IsAll       bool   `json:"isAll" bson:"isAll"`
	CreatedAt   int64  `json:"createdAt" bson:"createdAt"`
	LastSeen    int64  `json:"lastSeen" bson:"lastSeen"`
}
