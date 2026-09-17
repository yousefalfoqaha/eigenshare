package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"eigenshare/internal/app/views"
	"eigenshare/internal/apperr"
	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func (app *App) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (app *App) loginPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "login.html", "login", nil, nil)
}

func (app *App) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.render(w, r, "login.html", "login", nil, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}
	cred := auth.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := app.Auth.Login(cred)
	if err != nil {
		app.render(w, r, "login.html", "login", nil, err)
		return
	}
	SetSessionCookie(w, session.Token, 60*24*7)
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (app *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if err := app.Auth.Logout(cookie.Value); err != nil {
			app.clientError(w, r, err)
			return
		}
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (app *App) usersPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "users.html", "users", nil, nil)
}

func (app *App) newUserPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "user_form.html", "new user", &views.UserFormContent{}, nil)
}

func (app *App) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.render(w, r, "user_form.html", "new user", &views.UserFormContent{}, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := app.User.CreateUser(req); err != nil {
		app.render(w, r, "user_form.html", "new user", &views.UserFormContent{}, err)
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (app *App) editUserPage(w http.ResponseWriter, r *http.Request) {
	u, err := app.User.GetUser(r.PathValue("id"))
	if err != nil {
		app.render(w, r, "user_form.html", "edit user", &views.UserFormContent{}, err)
		return
	}
	app.render(w, r, "user_form.html", "edit user", &views.UserFormContent{User: u}, nil)
}

func (app *App) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v := &views.UserFormContent{}
	if u, getErr := app.User.GetUser(id); getErr == nil {
		v.User = u
	}
	title := "edit user"
	if v.User != nil {
		title = v.User.Username
	}

	if err := r.ParseForm(); err != nil {
		app.render(w, r, "user_form.html", title, v, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := app.User.UpdateUser(id, auth.UserFromContext(r.Context()).ID, req); err != nil {
		app.render(w, r, "user_form.html", title, v, err)
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (app *App) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := app.User.DeleteUser(r.PathValue("id"), auth.UserFromContext(r.Context()).ID); err != nil {
		app.render(w, r, "users.html", "users", nil, err)
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (app *App) sharesPage(w http.ResponseWriter, r *http.Request) {
	v := &views.SharesContent{}
	shares, err := app.Share.GetShares(auth.UserFromContext(r.Context()))
	if err != nil {
		app.render(w, r, "shares.html", "shares", v, err)
		return
	}
	v.Shares = shares
	progress := make(map[string]int64, len(shares))
	for _, s := range shares {
		written, err := app.Share.GetShareProgress(s.ID, auth.UserFromContext(r.Context()))
		if err != nil {
			app.render(w, r, "shares.html", "shares", v, err)
			return
		}
		var total int64
		for _, n := range written {
			total += n
		}
		progress[s.ID] = total
	}
	v.Progress = progress
	app.render(w, r, "shares.html", "shares", v, nil)
}

func (app *App) trashPage(w http.ResponseWriter, r *http.Request) {
	v := &views.SharesContent{}
	shares, err := app.Share.GetTrashedShares(auth.UserFromContext(r.Context()))
	if err != nil {
		app.render(w, r, "trash.html", "trash", v, err)
		return
	}
	v.Shares = shares
	app.render(w, r, "trash.html", "trash", v, nil)
}

func (app *App) newSharePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "new_share.html", "new share", nil, nil)
}

func (app *App) sharePage(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	sh, err := app.Share.GetShareByHandle(handle, auth.UserFromContext(r.Context()))
	if err != nil {
		app.render(w, r, "share.html", handle, &views.ShareContent{}, err)
		return
	}
	app.render(w, r, "share.html", sh.Handle, &views.ShareContent{Share: sh}, nil)
}

func (app *App) editSharePage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), nil)
}

func (app *App) trashShare(w http.ResponseWriter, r *http.Request) {
	if err := app.Share.TrashShare(r.PathValue("id"), auth.UserFromContext(r.Context())); err != nil {
		app.render(w, r, "shares.html", "shares", &views.SharesContent{}, err)
		return
	}
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (app *App) restoreShare(w http.ResponseWriter, r *http.Request) {
	if err := app.Share.RecoverShare(r.PathValue("id"), auth.UserFromContext(r.Context())); err != nil {
		app.render(w, r, "trash.html", "trash", &views.SharesContent{}, err)
		return
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
}

func (app *App) deleteShares(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.render(w, r, "trash.html", "trash", &views.SharesContent{}, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}
	if err := app.Share.Delete(auth.UserFromContext(r.Context()), r.PostForm["shareID"]...); err != nil {
		app.render(w, r, "trash.html", "trash", &views.SharesContent{}, err)
		return
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
}

func (app *App) deleteFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.Share.DeleteFile(id, r.PathValue("fileID"), auth.UserFromContext(r.Context())); err != nil {
		app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), err)
		return
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
}

func (app *App) addOwner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), apperr.New(http.StatusBadRequest, err.Error()))
		return
	}

	if err := app.Share.AddOwner(id, auth.UserFromContext(r.Context()), r.PostForm.Get("userID")); err != nil {
		app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), err)
		return
	}

	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
}

func (app *App) removeOwner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.Share.RemoveOwner(id, auth.UserFromContext(r.Context()), r.PathValue("ownerID")); err != nil {
		app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), err)
		return
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
}

func (app *App) setVisibility(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), apperr.New(http.StatusBadRequest, err.Error()))
		return
	}

	visibility := share.Visibility(r.PostForm.Get("visibility"))
	if err := app.Share.SetVisibility(id, auth.UserFromContext(r.Context()), visibility); err != nil {
		app.render(w, r, "edit_share.html", "edit share", views.NewEditShareContent(app.Share), err)
		return
	}

	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
}

func (app *App) addFilesToShare(w http.ResponseWriter, r *http.Request) {
	shareID := r.PathValue("id")

	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.clientError(w, r, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}

	newFiles, err := app.Share.UploadFiles(shareID, req.Files, auth.UserFromContext(r.Context()))
	if err != nil {
		app.clientError(w, r, err)
		return
	}

	_ = writeAPIResponse(w, http.StatusOK, newFiles)
}

func (app *App) createShare(w http.ResponseWriter, r *http.Request) {
	var req share.ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.clientError(w, r, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}

	sh, err := app.Share.RequestShare(req, auth.UserFromContext(r.Context()))
	if err != nil {
		app.clientError(w, r, err)
		return
	}

	_ = writeAPIResponse(w, http.StatusCreated, sh)
}

func (app *App) streamFile(w http.ResponseWriter, r *http.Request) {
	shareID := r.PathValue("id")
	fileID := r.PathValue("fileID")

	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		app.clientError(w, r, apperr.New(http.StatusBadRequest, "invalid Upload-Offset header"))
		return
	}

	bytesWritten, err := app.Share.StreamFile(shareID, fileID, offset, r.Body, auth.UserFromContext(r.Context()))
	if err != nil {
		var conflict *share.ChunkOffsetConflictError
		if errors.As(err, &conflict) {
			w.Header().Set("Upload-Offset", strconv.FormatInt(conflict.CurrentOffset, 10))
		}
		app.clientError(w, r, err)
		return
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (app *App) downloadAll(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	sh, err := app.Share.GetShareByHandle(handle, auth.UserFromContext(r.Context()))
	if err != nil {
		app.clientError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="share.zip"`)
	if err := app.Share.DownloadAll(sh.ID, w); err != nil {
		app.clientError(w, r, err)
	}
}

func (app *App) downloadFile(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	fileID := r.PathValue("fileID")

	sh, err := app.Share.GetShareByHandle(handle, auth.UserFromContext(r.Context()))
	if err != nil {
		app.clientError(w, r, err)
		return
	}

	filePath, err := app.Share.DownloadFile(sh.ID, fileID)
	if err != nil {
		app.clientError(w, r, err)
		return
	}

	http.ServeFile(w, r, filePath)
}
