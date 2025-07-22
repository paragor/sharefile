package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/paragor/sharefile/internal/log"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

const impersonateCookieName = "impersonate-email"

type AuthOidcConfig struct {
	ClientId     string
	ClientSecret string
	IssuerUrl    string
	CookieKey    string
	Scopes       []string
	AllowedGroup string
	AdminGroup   string
}

func (c *AuthOidcConfig) Validate() error {
	if c.ClientId == "" {
		return fmt.Errorf("client id shoud not be empty")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("client secret shoud not be empty")
	}
	if c.IssuerUrl == "" {
		return fmt.Errorf("issuer url shoud not be empty")
	}
	if c.CookieKey == "" {
		return fmt.Errorf("cookie key shoud not be empty")
	}
	if len(c.Scopes) == 0 {
		return fmt.Errorf("scopes shoud not be empty")
	}
	return nil
}

type authOidcContext struct {
	cfg                    *AuthOidcConfig
	provider               rp.RelyingParty
	successRedirectPath    string
	idTokenCookieName      string
	refreshTokenCookieName string
}

func newOidcContext(cfg *AuthOidcConfig, callbackUrl string, successRedirectPath string) (*authOidcContext, error) {
	cookieHandler := httphelper.NewCookieHandler([]byte(cfg.CookieKey), []byte(cfg.CookieKey))
	options := []rp.Option{
		rp.WithCookieHandler(cookieHandler),
		rp.WithVerifierOpts(rp.WithIssuedAtOffset(5 * time.Second)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	provider, err := rp.NewRelyingPartyOIDC(ctx, cfg.IssuerUrl, cfg.ClientId, cfg.ClientSecret, callbackUrl, cfg.Scopes, options...)
	if err != nil {
		return nil, fmt.Errorf("error creating provider %v", err)
	}
	return &authOidcContext{
		cfg:                    cfg,
		provider:               provider,
		idTokenCookieName:      "oidc_id_token",
		refreshTokenCookieName: "oidc_refresh_token",
		successRedirectPath:    successRedirectPath,
	}, nil
}

func (oc *authOidcContext) checkAuthorizationByOidc(writer http.ResponseWriter, request *http.Request) (bool, *oidc.IDTokenClaims) {
	idToken, err := oc.getIdToken(request)
	if err != nil {
		idToken, err = oc.refreshTokensAndGetIdToken(writer, request)
		if err != nil {
			return false, nil
		}
		log.FromContext(request.Context()).Info("refresh token was success")
	}

	return oc.isAccessAllowed(idToken), idToken
}
func (oc *authOidcContext) getIdToken(request *http.Request) (*oidc.IDTokenClaims, error) {
	idToken, err := oc.provider.CookieHandler().CheckCookie(request, oc.idTokenCookieName)
	if err != nil || idToken == "" {
		return nil, fmt.Errorf("no id token in request")
	}
	claims, err := rp.VerifyIDToken[*oidc.IDTokenClaims](request.Context(), idToken, oc.provider.IDTokenVerifier())
	if err != nil {
		return nil, fmt.Errorf("error on extracting oidc token: %s", err)
	}
	return claims, nil
}

func (oc *authOidcContext) refreshTokensAndGetIdToken(w http.ResponseWriter, r *http.Request) (*oidc.IDTokenClaims, error) {
	refreshToken, err := oc.provider.CookieHandler().CheckCookie(r, oc.refreshTokenCookieName)
	if err != nil || refreshToken == "" {
		return nil, fmt.Errorf("no refresh token in request")
	}

	tokens, err := oc.provider.OAuthConfig().TokenSource(r.Context(), &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return nil, fmt.Errorf("error on refresh tokens: %s", err)
	}

	idToken := tokens.Extra("id_token").(string)
	if idToken == "" {
		return nil, fmt.Errorf("empty id token after refresh")
	}

	claims, err := rp.VerifyIDToken[*oidc.IDTokenClaims](r.Context(), idToken, oc.provider.IDTokenVerifier())
	if err != nil {
		return nil, fmt.Errorf("error on extracting oidc token: %s", err)
	}

	if err := oc.provider.CookieHandler().SetCookie(w, oc.idTokenCookieName, idToken); err != nil {
		return nil, fmt.Errorf("cant set id token cookie: %w", err)
	}
	if err := oc.provider.CookieHandler().SetCookie(w, oc.refreshTokenCookieName, tokens.RefreshToken); err != nil {
		return nil, fmt.Errorf("cant set refresh token cookie: %w", err)
	}

	return claims, nil
}

func (oc *authOidcContext) isAccessAllowed(token *oidc.IDTokenClaims) bool {
	if token.Email == "" || !token.EmailVerified {
		return false
	}
	if oc.cfg.AllowedGroup == "" {
		return true
	}

	groups, err := oc.extractGroups(token)
	if err != nil {
		rawToken, _ := token.MarshalJSON()
		log.FromContext(context.Background()).
			With(slog.String("id_token", string(rawToken))).
			With(log.Error(err)).
			Error("cant check allowance access")
		return false
	}

	for _, group := range groups {
		if group == oc.cfg.AllowedGroup {
			return true
		}
	}

	return false
}

func (oc *authOidcContext) isAdmin(token *oidc.IDTokenClaims) bool {
	if !oc.isAccessAllowed(token) {
		return false
	}

	groups, err := oc.extractGroups(token)
	if err != nil {
		rawToken, _ := token.MarshalJSON()
		log.FromContext(context.Background()).
			With(slog.String("id_token", string(rawToken))).
			With(log.Error(err)).
			Error("cant check admin access")
		return false
	}

	for _, group := range groups {
		if group == oc.cfg.AdminGroup {
			return true
		}
	}

	return false
}

func (oc *authOidcContext) extractGroups(token *oidc.IDTokenClaims) ([]string, error) {
	groups := token.Claims["groups"]
	if groups == nil {
		return nil, fmt.Errorf("groups claim is empty in oath id token")
	}

	groupsInterfaces, ok := groups.([]interface{})
	if !ok {
		return nil, fmt.Errorf(
			"groups claim is not list of strings in oath id token, actual type: %T `%v`",
			groups, groups,
		)
	}
	result := make([]string, 0, len(groupsInterfaces))
	for _, group := range groupsInterfaces {
		group, ok := group.(string)
		if !ok {
			return nil, fmt.Errorf(
				"group claim is not strings in oath id token, actual type: %T `%v`",
				group, group,
			)
		}
		result = append(result, group)
	}

	return result, nil
}

func (oc *authOidcContext) userInfoCallback(
	w http.ResponseWriter,
	r *http.Request,
	tokens *oidc.Tokens[*oidc.IDTokenClaims],
	_ string,
	_ rp.RelyingParty,
	info *oidc.UserInfo,
) {
	claim, err := rp.VerifyIDToken[*oidc.IDTokenClaims](r.Context(), tokens.IDToken, oc.provider.IDTokenVerifier())
	if err != nil {
		httpError(r.Context(), w, "cant verify id token", err, http.StatusUnauthorized)
		return
	}

	if !oc.isAccessAllowed(claim) {
		httpError(r.Context(), w, "email is blocked", fmt.Errorf(
			"email '%s' is blocked",
			info.Email,
		), http.StatusUnauthorized)
		return
	}
	if err := oc.provider.CookieHandler().SetCookie(w, oc.idTokenCookieName, tokens.IDToken); err != nil {
		httpError(r.Context(), w, "cant set id token cookie", err, http.StatusInternalServerError)
		return
	}
	if err := oc.provider.CookieHandler().SetCookie(w, oc.refreshTokenCookieName, tokens.RefreshToken); err != nil {
		httpError(r.Context(), w, "cant set refresh token cookie", err, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, oc.successRedirectPath, http.StatusFound)
}

func (oc *authOidcContext) AuthCallbackHandler() http.Handler {
	return rp.CodeExchangeHandler(rp.UserinfoCallback(oc.userInfoCallback), oc.provider)
}

func (oc *authOidcContext) AuthLoginHandler() http.Handler {
	return rp.AuthURLHandler(func() string { return uuid.New().String() }, oc.provider)
}

func (s *httpServer) apiLogout(w http.ResponseWriter, r *http.Request) {
	for _, cookie := range []string{
		s.oidc.idTokenCookieName,
		s.oidc.refreshTokenCookieName,
		impersonateCookieName,
	} {
		http.SetCookie(w, &http.Cookie{
			Name:     cookie,
			Path:     "/",
			MaxAge:   -1,
			Secure:   true,
			HttpOnly: true,
		})
	}
	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *httpServer) AuthMiddleware() mux.MiddlewareFunc {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			authPass, claim := s.oidc.checkAuthorizationByOidc(writer, request)
			if !authPass {
				s.htmxPageLogin(writer, request)
				return
			}
			auth := &authContext{
				Email:    claim.Email,
				IsAdmin:  s.oidc.isAdmin(claim),
				RawToken: claim,
				ExpireAt: claim.GetExpiration(),
			}
			if auth.IsAdmin {
				for _, cookie := range request.Cookies() {
					if cookie.Name == impersonateCookieName {
						auth.ImpersonateEmail = cookie.Value
					}
				}
			}

			ctx := request.Context()
			ctx = context.WithValue(ctx, authContextKeyValue, auth)
			logger := log.FromContext(ctx).With(
				slog.String("user", auth.Email),
				slog.String("impersonate_as_user", auth.ImpersonateEmail),
				slog.Bool("is_admin", auth.IsAdmin),
			)
			logger.Debug("auth middleware is successfully passed")
			ctx = log.PutIntoContext(ctx, logger)
			handler.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

type authContext struct {
	Email            string
	IsAdmin          bool
	ImpersonateEmail string
	ExpireAt         time.Time

	RawToken any
}
type authContextKey struct{}

var authContextKeyValue = authContextKey{}

func (s *httpServer) extractAuthContext(request *http.Request) (*authContext, error) {
	authContextValue := request.Context().Value(authContextKeyValue)
	if authContextValue == nil {
		return nil, fmt.Errorf("no auth context in request context")
	}
	obj, ok := authContextValue.(*authContext)
	if !ok {
		return nil, fmt.Errorf("auth context in request with wrong value: %T", authContextValue)
	}
	return obj, nil
}
func (s *httpServer) extractEmail(request *http.Request) (string, error) {
	auth, err := s.extractAuthContext(request)
	if err != nil {
		return "", err
	}
	if auth.IsAdmin && auth.ImpersonateEmail != "" {
		return auth.ImpersonateEmail, nil
	}
	return auth.Email, nil
}
