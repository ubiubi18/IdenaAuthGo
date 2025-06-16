# Frontend Eligibility Check

The current `static/index.html` implements the eligibility check using a small
inline script:

1. Users enter an address in an input field and click the *Check* button.
2. The script calls `/whitelist/check?address=ADDRESS` via `fetch`.
3. The JSON response is parsed and a simple browser alert displays either
   "Eligible!" or "Not eligible" based on the `eligible` field.

No additional validation is performed client‑side—the decision is entirely based
on the backend response. There is also a `signinFallback()` that redirects to
`/signin` when no address is entered.

## Suggestions

* Show a loading indicator while the request is in flight.
* Display errors returned by the API (e.g. malformed address or server errors)
  instead of failing silently.
* Replace the browser `alert()` with inline status text so the page feels more
  integrated.
* Cache the fetched Merkle root to avoid an extra request every page load.
