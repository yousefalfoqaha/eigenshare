package web

import (
	"net/http"

	"eigenshare/internal/user"
)

func (c *Controller) UsersPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "users.html", nil)
}

func (c *Controller) NewUserPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "user_form.html", nil)
}

func (c *Controller) CreateUser(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return ErrInvalidRequest
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userSvc.CreateUser(req); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "user created")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) EditUserPage(w http.ResponseWriter, r *http.Request) error {
	u, err := c.userSvc.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	return c.templs.renderPage(w, r, http.StatusOK, "user_form.html", u)
}

func (c *Controller) UpdateUser(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return ErrInvalidRequest
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userSvc.UpdateUser(r.PathValue("id"), userFromContext(r).ID, req); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "user updated")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) DeleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := c.userSvc.DeleteUser(r.PathValue("id"), userFromContext(r).ID); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "user deleted")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}
