[Table of contents](./README.md#table-of-contents)

Authentication and access delegations
=====================================

Introduction
------------

In this document, we will cover how to protect the usage of the cozy-stack.
When the cozy-stack receives a request, it checks that the request is
authorized, and if yes, it processes it and answers it.


What about OAuth2?
------------------

OAuth2 is about delegating an access to resources on a server to another
party. It is a framework, not a strictly defined protocol, for organizing the
interactions between these 4 actors:

- the resource owner, the "user" that can click on buttons
- the client, the website or application that would like to access the
  resources
- the authorization server, whose role is limited to give tokens but is
  central in OAuth2 interactions
- the resources server, the server that controls the resources.

For cozy, both the authorization server and the resources server roles are
played by the cozy-stack. The resource owner is the owner of a cozy instance.
The client can be the cozy-desktop app, cozy-mobile, or many other
applications.

OAuth2, and its extensions, is a large world. At its core, there is 2 things:
letting the client get a token issued by the authorization server, and using
this token to access to the resources. OAuth2 describe 4 flows, called grant
types, for the first part:

- Authorization code
- Implicit grant type
- Client credentials grant type
- Resource owner credentials grant type.

On cozy, only the most typical one is used: authorization code. To start this
flow, the client must have a `client_id` and `client_secret`. The Cozy stack
implements the OAuth2 Dynamic Client Registration Protocol (an extension to
OAuth2) to allow the clients to obtain them.

OAuth2 has also 3 ways to use a token:

- in the query-string (even if the spec does not recommended it)
- in the POST body
- in the HTTP Authorization header.

On cozy, only the HTTP header is supported.

OAuth2 has a lot of assumptions. Let's see some of them and their consequences
on Cozy:

- TLS is very important to secure the communications. in OAuth 1, there was a
  mechanism to sign the requests. But it was very difficult to get it right
  for the developers and was abandonned in OAuth2, in favor of using TLS. The
  Cozy instance are already accessible only in HTTPS, so there is nothing
  particular to do for that.

- There is a principle called TOFU, Trust On First Use. It said that if the
  user will give his permission for delegating access to its resources when
  the client will try to access them for the first time. Later, the client
  will be able to keep accessing them even if the user is no longer here to
  give his permissions.

- The client can't make the assumptions about when its tokens will work. The
  tokens have no meaning for him (like cookies in a browser), they are just
  something it got from the authorization server and can send with its
  request. The access token can expire, the user can revoke them, etc.

- OAuth 2.0 defines no cryptographic methods. But a developer that want to use
  it will have to put her hands in that.

If you want to learn OAuth 2 in details, I recommend the [OAuth 2 in Action
book](https://www.manning.com/books/oauth-2-in-action).


The cozy stack as an authorization server
-----------------------------------------

### GET /auth/login

Display a form with a password field to let the user authenticates herself to
the cozy stack.

This endpoint accepts a `redirect` parameter. If the user is already logged
in, she will be redirected immediately. Else, the parameter will be transfered
in the POST. This parameter can only contain a link to an application
installed on the cozy (thus to a subdomain of the cozy instance). To protect
against stealing authorization code with redirection, the fragment is always
overriden:

```http
GET /auth/login?redirect=https://contacts.cozy.example.org/foo?bar#baz HTTP/1.1
Host: cozy.example.org
Cookies: ...
```

**Note**: the redirect parameter should be URL-encoded. We haven't done that
to make it clear what the path (`foo`), the query-string (`bar`), and the
fragment (`baz`) are.

```http
HTTP/1.1 302 Moved Temporarily
Location: https://contacts.cozy.example.org/foo?bar#_=_
```

If the `redirect` parameter is invalid, the response will be `400 Bad
Request`. Same for other parameters, the redirection will happen only on
success (even if OAuth2 says the authorization server can redirect on errors,
it's very complicated to do it safely, and it is better to avoid this trap).

### POST /auth/login

After the user has typed her password and clicked on `Login`, a request is
made to this endpoint.

The `redirect` parameter is passed inside the body. If it is missing, the
redirection will be made against the default target: the home application of
this cozy instance.

```http
POST /auth/login HTTP/1.1
Host: cozy.example.org
Content-type: application/x-www-form-urlencoded

password=p4ssw0rd&redirect=https%3A%2F%2Fcontacts.cozy.example.org
```

```http
HTTP/1.1 302 Moved Temporarily
Set-Cookie: ...
Location: https://contacts.cozy.example.org/foo
```

### DELETE /auth/login

This can be used to log-out the user. A private context must be passed in the
query-string, to protect against CSRF attack on this (this can part of bigger
attacks like session fixation).

```http
DELETE /auth/login?CtxToken=token-for-a-private-context HTTP/1.1
Host: cozy.example.org
```

### POST /auth/register

This route is used by OAuth2 clients to dynamically register them-selves.

See [OAuth 2.0 Dynamic Client Registration
Protocol](https://tools.ietf.org/html/rfc7591) for the details.

It gives to the client these informations:

- `client_id`
- `client_secret`
- `registration_access_token`

### GET /auth/register/:client-id
### PUT /auth/register/:client-id
### DELETE /auth/register/:client-id

These routes follow the [OAuth 2.0 Dynamic Client Registration Management
Protocol RFC](https://tools.ietf.org/html/rfc7592). They allow an OAuth2
client to get back its metadata, update them, and unregister itself.

The client has to sent its registration access token to be able to use this
endpoint.

### GET /auth/authorize

When an OAuth2 client wants to get access to the data of the cozy owner, it
starts the OAuth2 dance with this step. The user is shown what the client asks
and has an accept button if she is OK with that.

The parameters are:

- `client_id`, that identify the client
- `redirect_uri`, it has be exactly the same of the one used in registration
- `state`, it's a protection against CSRF on the client (a random string
  generated by the client, that it can check when the user will be redirected
  with the authorization code. It can be used as a key in local storage for
  storing a state in a SPA).
- `response_type`, only `code` is supported
- `scope`, a space separated list of the permissions asked (a permission being
  formatted as `key:access`, like `files/images:read`).

```http
GET /auth/authorize?client_id=oauth-client-1&response_type=code&scope=files/images:read%20data/io.cozy.contacts:read&state=Eh6ahshepei5Oojo&redirect_uri=https%3A%2F%2Fclient.org%2F HTTP/1.1
Host: cozy.example.org
```

**Note** we follow the TOFU principle (Trust On First Use). It means that if
the user has already said yes for this authorization and scopes, she will be
redirected to the app directly. As for `/auth/login`, the fragment is
overriden in the redirection with `#_=_`.

**Note** we warn the user that he is about to share his data with an
application which only the callback URI is guaranteed.

### POST /auth/authorize

When the user accepts, her browser send a request to this endpoint:

```http
POST /auth/authorize
Host: cozy.example.org
Content-type: x-www-form-urlencoded

approve=Approve&csrf-token=johw6Sho
```

**Note**: this endpoint is protected against CSRF attacks.

The user is then redirected to the original client, with an access code in the
URL:

```http
HTTP/1.1 302 Moved Temporarily
Location: https://client.org/?state=Eh6ahshepei5Oojo&access_code=Aih7ohth
```

### POST /auth/access_token

Now, the client can check that the state is correct, and if it is the case,
ask for an `access_token`. It can use this route with the `code` (ie
access_code) given above.

This endpoint is also used to refresh the access token, by sending the
`refresh_token` instead of the `access_code`.

The parameters are:

- `grant_type`, with `authorization_code` or `refresh_token` as value
- `code` or `refresh_token`, depending on which grant type is used
- `client_id`
- `client_secret`

Example:

```http
POST /auth/access_token
Host: cozy.example.org
Content-type: x-www-form-urlencoded
Accept: application/json

grant_type=authorization_code&code=Aih7ohth&client_id=oauth-client-1&client_secret=Oung7oi5
```

```http
HTTP/1.1 200 OK
Content-type: application/json

{
  "access_token": "ooch1Yei",
  "token_type": "bearer",
  "refresh_token": "ui0Ohch8",
  "scope": "files/images:read data/io.cozy.contacts:read"
}
```

### FAQ

> What format is used for tokens?

The tokens are formatted as [JSON Web Tokens (JWT)](https://jwt.io/).

> What happens when the user has lost her password?

She can reset it from the command-line, like this:

```sh
$ cozy-stack instances reset-password cozy.example.org
ek0Jah1R
```

A new password is generated and print in the console.

> Is two-factor authentication (2FA) possible?

Yes, it's possible.

**TODO:** explain how


Client-side apps
----------------

**Important**: OAuth2 is not used here! The steps looks similar (like obtaining
a token), but when going in the details, it doesn't match.

### How to register the application?

The application is registered at install. See [app management](apps.md) for
details.

### How to get a token?

When a user access an application, she first loads the HTML page. Inside this
page, the `<body>` tag has a `data-cozy-token` attribute with a token. This
token is specific to a context, that can be either public or private.

We have prefered our custom solution to the implicit grant type of OAuth2 for
2 reasons:

1. It has a better User Experience. The implicit grant type works with 2
redirections (the application to the stack, and then the stack to the
application), and the first one needs JS to detect if the token is present or
not in the fragment hash. It has a strong impact on the time to load the
application.

2. The implicit grant type of OAuth2 has a severe drawback on security: the
token appears in the URL and is shown by the browser. It can also be leaked
with the HTTP `Referer` header.

### How to use a token?

The token can be sent to the cozy-stack in the query-string, like this:

```http
GET /data/io.cozy.events/6494e0ac-dfcb-11e5-88c1-472e84a9cbee?CtxToken=e7af77ba2c2dbe2d
HOST: cozy.example.org
```

If the user is authenticated, her cookies will be sent automatically. The
cookies are needed for a token to a private context to be valid.

### How to refresh a token?

The context token is valid only for 24 hours. If the application is opened for
more than that, it will need to get a new token. But most applications won't
be kept open for so long and it's okay if they don't try to refresh tokens. At
worst, the user just had to reload its page and it will work again.

The app can know it's time to get a new token when the stack starts sending
401 Unauthorized responses. In that case, it can fetches the same html page
that it was loaded initially, parses it and extracts the new token.


Third-party websites
--------------------

### How to register the application?

If a third-party websites would like to access a cozy, it had to register
first. For example, a big company can have data about a user and may want
to offer her a way to get her data back in her cozy. When the user is
connected on the website of this company, she can give her cozy address. The
website will then register on this cozy, using the OAuth2 Dynamic Client
Registration Protocol, as explained [above](#post-authregister).

### How to get a token?

To get an access token, it's enough to follow the authorization code flow of
OAuth2:

- sending the user to the cozy, on the authorize page
- if the user approves, she is then redirected back to the client
- the client gets the access code and can exchange it to an access token.

### How to use a token?

The access token can be sent as a bearer token, in the Authorization header of
HTTP:

```http
GET /data/io.cozy.contacts/6494e0ac-dfcb-11e5-88c1-472e84a9cbee HTTP/1.1
Host: cozy.example.org
Accept: application/json
Authorization: bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9.TJVA95OrM7E2cBab30RMHrHDcEfxjoYZgeFONFh7HgQ
```

### How to refresh a token?

The access token will be valid only for 24 hours. After that, a new access
token must be asked. To do that, just follow the refresh token flow, as
explained [above](#post-authaccess_token).


Devices and browser extensions
------------------------------

For devices and browser extensions, it is nearly the same than for third-party
websites. The main difficulty is the redirect_uri. In OAuth2, the access code
is given to the client by redirecting the user to an URL controlled by the
client. But devices and browser extensions don't have an obvious URL for that.

The IETF has published an RFC called [OAuth 2.0 for Native
Apps](https://tools.ietf.org/html/draft-ietf-oauth-native-apps-05).

### Native apps on desktop

A desktop native application can start an embedded webserver on localhost. The
redirect_uri will be something like `http://127.0.0.1:19856/callback`.

### Native apps on mobile

On mobile, the native apps can often register a custom URI scheme, like
`com.example.oauthclient:/`. Just be sure that no other app has registered
itself with the same URI.

### Chrome extensions

Chrome extensions can use URL like
`https://<extension-id>.chromiumapp.org/<anything-here>` for their usage.
See https://developer.chrome.com/apps/app_identity#non for more details. It
has also a method to simplify the creation of such an URL:
[`chrome.identity.getRedirectURL`](https://developer.chrome.com/apps/identity#method-getRedirectURL).

### Firefox extensions

It is possible to use an _out of band_ URN: `urn:ietf:wg:oauth:2.0:oob:auto`.
The token is then extracted from the title of the page.
See [this addon for google
oauth2](https://github.com/AdrianArroyoCalle/firefox-addons/blob/master/addon-google-oauth2/addon-google-oauth2.js)
as an example.


Security considerations
-----------------------

The password will be stored in a secure fashion, with a password hashing
function. The hashing function and its parameter will be stored with the hash,
in order to make it possible to change the algorithm and/or the parameters
later if we had any suspicion that it became too weak. The initial algorithm
is [scrypt](https://godoc.org/golang.org/x/crypto/scrypt).

The access code is valid only once, and will expire after 5 minutes

Dynamically registered applications won't have access to all possible scopes.
For example, an application that has been dynamically registered can't ask the
cozy owner to give it the right to install other applications. This limitation
should improve security, as avoiding too powerful scopes to be used with
unknown applications.

The cozy stack will apply rate limiting to avoid brute-force attacks.

The cozy stack offers
[CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Access_control_CORS)
for most of its services. But it's disabled for `/auth` (it doesn't make sense
here) and for the client-side applications (to avoid leaking their tokens).

The client should really use HTTPS for its `redirect_uri` parameter, but it's
allowed to use HTTP for localhost, as in the native desktop app example.

OAuth2 says that the `state` parameter is optional in the authorization code
flow. But it is mandatory to use it with Cozy.

For more on this subject, here is a list of links:

- https://www.owasp.org/index.php/Authentication_Cheat_Sheet
- https://tools.ietf.org/html/rfc6749#page-53
- https://tools.ietf.org/html/rfc6819
- https://tools.ietf.org/html/draft-ietf-oauth-closing-redirectors-00
- http://www.oauthsecurity.com/


Conclusion
----------

Security is hard. If you want to share some concerns with us, do not hesitate
to send us an email to security AT cozycloud.cc.
