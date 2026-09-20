package app

import (
	"net/http"
	"net/url"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func (app *App) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (app *App) loginPage(w http.ResponseWriter, r *http.Request) {
	app.writePage(w, r, "login.html", nil)
}

func (app *App) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	cred := auth.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := app.Auth.Login(cred)
	if err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/login?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	SetSessionCookie(w, session.Token, 60*24*7)
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (app *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if err := app.Auth.Logout(cookie.Value); err != nil {
			_, message := errorResponse(r, err)
			http.Redirect(w, r, "/login?err="+url.QueryEscape(message), http.StatusSeeOther)
			return
		}
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (app *App) usersPage(w http.ResponseWriter, r *http.Request) {
	users, err := app.User.GetAllUsers()
	if err != nil {
		app.pageError(w, r, err)
		return
	}
	app.writePage(w, r, "users.html", users)
}

func (app *App) newUserPage(w http.ResponseWriter, r *http.Request) {
	app.writePage(w, r, "user_form.html", nil)
}

func (app *App) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/users/new?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := app.User.CreateUser(req); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/users/new?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (app *App) editUserPage(w http.ResponseWriter, r *http.Request) {
	u, err := app.User.GetUser(r.PathValue("id"))
	if err != nil {
		app.pageError(w, r, err)
		return
	}
	app.writePage(w, r, "user_form.html", u)
}

func (app *App) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/users/"+id+"/edit?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := app.User.UpdateUser(id, auth.UserFromContext(r.Context()).ID, req); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/users/"+id+"/edit?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (app *App) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := app.User.DeleteUser(r.PathValue("id"), auth.UserFromContext(r.Context()).ID); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/users?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (app *App) sharesPage(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFromContext(r.Context())

	shares, err := app.Share.GetShares(actor)
	if err != nil {
		app.pageError(w, r, err)
		return
	}

	progress := make(map[string]int64, len(shares))
	for _, s := range shares {
		written, err := app.Share.GetShareProgress(s.ID, actor)
		if err != nil {
			app.pageError(w, r, err)
			return
		}
		var total int64
		for _, n := range written {
			total += n
		}
		progress[s.ID] = total
	}

	users, _ := app.User.GetAllUsers()
	namesByID := make(map[string]string, len(users))
	for _, u := range users {
		namesByID[u.ID] = u.Username
	}

	ownerNames := make(map[string][]string, len(shares))
	for _, s := range shares {
		names := make([]string, 0, len(s.Owners))
		for _, id := range s.Owners {
			if name, ok := namesByID[id]; ok {
				names = append(names, name)
			}
		}
		ownerNames[s.ID] = names
	}

	app.writePage(w, r, "shares.html", &struct {
		Shares     []share.ShareSummary
		Progress   map[string]int64
		OwnerNames map[string][]string
	}{shares, progress, ownerNames})
}

func (app *App) trashPage(w http.ResponseWriter, r *http.Request) {
	shares, err := app.Share.GetTrashedShares(auth.UserFromContext(r.Context()))
	if err != nil {
		app.pageError(w, r, err)
		return
	}
	app.writePage(w, r, "trash.html", shares)
}

func (app *App) newSharePage(w http.ResponseWriter, r *http.Request) {
	app.writePage(w, r, "new_share.html", nil)
}

func (app *App) sharePage(w http.ResponseWriter, r *http.Request) {
	sh, err := app.Share.GetShareByHandle(r.PathValue("handle"), auth.UserFromContext(r.Context()))
	if err != nil {
		app.pageError(w, r, err)
		return
	}
	app.writePage(w, r, "share.html", sh)
}

func (app *App) editSharePage(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFromContext(r.Context())

	sh, err := app.Share.GetShare(r.PathValue("id"), actor)
	if err != nil {
		app.pageError(w, r, err)
		return
	}

	progress, _ := app.Share.GetShareProgress(sh.ID, actor)
	users, _ := app.User.GetAllUsers()

	tab := r.URL.Query().Get("tab")
	if tab != "owners" {
		tab = "files"
	}

	ownerNames := make([]string, 0, len(sh.Owners))
	for _, u := range users {
		if sh.IsOwner(u) {
			ownerNames = append(ownerNames, u.Username)
		}
	}

	app.writePage(w, r, "edit_share.html", &struct {
		Share      *share.Share
		Progress   map[string]int64
		Users      []*user.User
		Tab        string
		OwnerNames []string
	}{sh, progress, users, tab, ownerNames})
}

func (app *App) trashShare(w http.ResponseWriter, r *http.Request) {
	if err := app.Share.TrashShare(r.PathValue("id"), auth.UserFromContext(r.Context())); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/shares?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (app *App) restoreShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dest := "/trash"
	if r.PostFormValue("redirect") == "edit" {
		dest = "/shares/" + id + "/edit"
	}
	if err := app.Share.RecoverShare(id, auth.UserFromContext(r.Context())); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, dest+"?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func (app *App) deleteShares(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/trash?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := app.Share.Delete(auth.UserFromContext(r.Context()), r.PostForm["shareID"]...); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/trash?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
}

func (app *App) deleteFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.Share.DeleteFile(id, r.PathValue("fileID"), auth.UserFromContext(r.Context())); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/shares/"+id+"/edit?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
}

func (app *App) addOwner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners&err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := app.Share.AddOwner(id, auth.UserFromContext(r.Context()), r.PostForm.Get("userID")); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners&err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusFound)
}

func (app *App) removeOwner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.Share.RemoveOwner(id, auth.UserFromContext(r.Context()), r.PathValue("ownerID")); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners&err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusFound)
}

func (app *App) setVisibility(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/shares/"+id+"/edit?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := app.Share.SetVisibility(id, auth.UserFromContext(r.Context()), share.Visibility(r.PostForm.Get("visibility"))); err != nil {
		_, message := errorResponse(r, err)
		http.Redirect(w, r, "/shares/"+id+"/edit?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
}

func (app *App) downloadAll(w http.ResponseWriter, r *http.Request) {
	sh, err := app.Share.GetShareByHandle(r.PathValue("handle"), auth.UserFromContext(r.Context()))
	if err != nil {
		app.pageError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="share.zip"`)
	if err := app.Share.DownloadAll(sh.ID, w); err != nil {
		app.pageError(w, r, err)
	}
}

func (app *App) downloadFile(w http.ResponseWriter, r *http.Request) {
	sh, err := app.Share.GetShareByHandle(r.PathValue("handle"), auth.UserFromContext(r.Context()))
	if err != nil {
		app.pageError(w, r, err)
		return
	}

	filePath, err := app.Share.DownloadFile(sh.ID, r.PathValue("fileID"))
	if err != nil {
		app.pageError(w, r, err)
		return
	}

	http.ServeFile(w, r, filePath)
}
