package user

type CreateRequest struct {
	Username string `form:"username" json:"username"`
	Password string `form:"password" json:"password"`
	IsAdmin  bool   `form:"isAdmin" json:"isAdmin"`
}

type UpdateRequest struct {
	Username string `form:"username" json:"username"`
	Password string `form:"password" json:"password"`
	IsAdmin  bool   `form:"isAdmin" json:"isAdmin"`
}
