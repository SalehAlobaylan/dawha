# Object storage: MinIO locally, Cloudflare R2 in production, one `S3Store` for both

**Status:** the adapter has been verified against a real S3 implementation once, on
one machine, against a local MinIO. It has never been pointed at Cloudflare R2. The
last section says plainly what that leaves unproven, because "the MinIO run was
green" is not a sentence about R2.

**Decision:** MinIO for local and integration verification, Cloudflare R2 in
production, one `S3Store` (`services/core-api/platform/storage/s3.go`) for both.
R2 speaks the S3 API, so this is a configuration decision and not a second adapter.
Renaming the adapter away from `S3` would be churn for its own sake.

**Decision it replaces:** plan 008 recorded the storage residual as "a deployment
must still run one real presign before trusting it". That presign has now been run,
against MinIO, by a test that runs on demand rather than by a person remembering.

**Sources.** Every claim about R2 below is from Cloudflare's own documentation, named
at the point of use. Nothing here is inferred from AWS's documentation, from the
adapter's behaviour, or from what "S3-compatible" usually means. Where a claim could
not be checked from Cloudflare's documentation it says **unverified** and says why.

## What the MinIO run found

Two disagreements between the adapter and a real S3 implementation. Both were
invisible to the test double, which is the argument for having run this at all.

### 1. `Put` failed against every plaintext S3 endpoint

Every upload through `S3Store` failed with:

```text
operation error S3: PutObject, compute input header checksum failed,
unseekable stream is not supported without TLS and trailing checksum
```

The cause was in the adapter, not in S3. `Put` wrapped the caller's `io.Reader` in a
counter to measure it, and the wrapper was not an `io.Seeker` - so a body that could
rewind, such as the `*bytes.Reader` the upload path always hands over, arrived at the
AWS SDK as a stream that could not. The SDK computes a request checksum by reading
`PutObject` bodies and rewinding them, and over plain HTTP it has no alternative: over
TLS it can send a trailing checksum while the stream goes past, over HTTP it cannot,
so the body has to be seekable. Same adapter, same body, same credentials: 403-free
`Get`, 403-free `Delete`, and a `Put` that never once reached the server.

The fix is in the adapter. `newPutBody` now settles the length before the call and
always hands the SDK something it can rewind: a seekable body is wrapped with a
forwarding `Read`/`Seek`, and a body that cannot seek is read into memory first. The
size reported is the length of the part that is actually sent, which is also a
correction: the old counter reported the reader's *offset* as the declared length,
and fell back to counting bytes across a call that the SDK may make more than once.

What this says about production: a deployment on an HTTPS endpoint (AWS, R2) would
probably never have seen this, because TLS takes the trailing-checksum path. It says
nothing good about plaintext endpoints, and plaintext is what a local MinIO, a
private-network MinIO and several S3-compatible services actually are.

### 2. A presigned URL is not a pure function of the key and the expiry

The contract suite asserted that two presigns of one key and expiry must be
byte-identical. The double has no clock, so that assertion could never fail there. A
SigV4 presigned URL carries `X-Amz-Date` and the signature covers it, so two presigns
of the same key and expiry that straddle a second *must* differ - and against MinIO
they do.

The local signer has the same property for a different reason: its deadline is a unix
second, so a presign that lands on the other side of a second boundary produces a
different URL too. The assertion was therefore wrong for both adapters and had been
passing by luck.

The contract now asserts what a signer can actually promise: the signature binds the
key, the signature binds the expiry, and two presigns of the same key and expiry are
either identical or differ in nothing but a signed clock reading and the signature
covering it. The double was given a clock and an `X-Amz-Date` so the second branch is
reachable without MinIO. Against a real endpoint the property is proved rather than
argued: `TestMinIOPresignedURLIsCheckedByTheServer` presigns twice 1.1 seconds apart -
a span longer than a second cannot avoid crossing one - and requires the difference to
be confined to the date and the signature.

## The configuration difference, as far as it is documented

| | MinIO, locally | Cloudflare R2, in production | Source |
| --- | --- | --- | --- |
| Endpoint | `http://localhost:59000` published by compose; `http://minio:9000` on the compose network | `https://<ACCOUNT_ID>.r2.cloudflarestorage.com`. A bucket created with a jurisdiction must be reached through that jurisdiction's host: `<ACCOUNT_ID>.eu.r2.cloudflarestorage.com`, `.fedramp.` or `.us.` | R2 docs, "Authentication" |
| Region | `us-east-1`, because the SDK needs a region to sign with and MinIO checks the credential scope against it | `auto`. An empty value and `us-east-1` both alias to `auto`, as does the `LocationConstraint` of `CreateBucket` | R2 docs, "S3 API compatibility" |
| Credentials | `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD`, the non-secret development defaults in `.env.example`, in the same spirit as `POSTGRES_PASSWORD=dawha_local` | An R2 API token used as an **Access Key ID** and a **Secret Access Key**. Either an Account token or a User token, optionally scoped to a set of buckets, with `Object Read & Write` or `Object Read only` the narrow permissions. The Secret Access Key is shown once | R2 docs, "Authentication" |
| Addressing | path style, required: virtual-hosted addressing needs a DNS record for the bucket name in a hostname, which `localhost` cannot have | **Unverified.** Cloudflare's documentation uses both forms - the presigned-URL example is virtual-hosted (`https://my-bucket.<ACCOUNT_ID>.r2.cloudflarestorage.com/...`), the AWS CLI example's output is path style (`https://<ACCOUNT_ID>.r2.cloudflarestorage.com/my-bucket/...`) - and the Go example sets no `UsePathStyle`, so it relies on the SDK's default. No page consulted states which style R2 requires | R2 docs, "Presigned URLs" and "aws-sdk-go" |
| Presigned URLs | SigV4 query parameters, verified by MinIO. Tampering one of them is `403 SignatureDoesNotMatch`; an unsigned read of the same object is `403 AccessDenied` | GET, HEAD, PUT and DELETE only; POST (multipart form upload) is not supported. Expiry from **1 second to 7 days (604,800 seconds)**. "The signature parameters cannot be tampered with. Attempting to modify the resource, operation, or expiry will result in a `403/SignatureDoesNotMatch` error." Presigned URLs work only on the S3 API domain, never on a custom domain, and one URL can be reused until it expires | R2 docs, "Presigned URLs" |
| `PutObject` | accepted, with the SDK's default CRC-64/NVME request checksum | implemented. CRC-64/NVME is supported as `FULL_OBJECT`; CRC-32, CRC-32C, SHA-1 and SHA-256 are supported **only** as `COMPOSITE`, and `Content-MD5` is supported | R2 docs, "S3 API compatibility" |
| Bucket policy | none attached by `make storage-up`, so an unsigned read is refused. That refusal is what makes the presigned read mean anything | **Unverified.** No R2 bucket exists yet, and Cloudflare's documentation on public buckets and access policies was not read for this record |

One reassuring detail fell out of reading those pages. The AWS SDK's presigned `GET`
puts `X-Amz-Checksum-Mode=ENABLED` and `x-id=GetObject` in the query string, and so
does the presigned URL Cloudflare documents as its worked example - so the URLs this
adapter hands out are shaped like the one R2 publishes rather than merely like
something that happens to verify.

Two of these interact with policy in this repository and need no change:

- The adapter's `MaxSignedURLExpiry` is 15 minutes and `NormalizeExpiry` refuses
  anything longer. R2's floor is 1 second and its ceiling is 7 days, so the adapter's
  window sits inside R2's with room on both sides. Nothing in the shared policy has to
  bend at cutover.
- The adapter requires `S3_REGION` and `S3_ENDPOINT` to be set explicitly. For R2 that
  is `auto` and the account host; leaving the endpoint unset means AWS, which is the
  one thing a deployment must not do by accident.

## The cutover checklist

Nothing on this list has been run. It is what must be run, once, against a real R2
bucket, before production traffic - and the evidence for each item is a value pasted
into the deployment change, not a tick.

1. **One presigned GET, fetched over HTTP by a client with no credentials and no
   SDK.** The same shape as `TestMinIOPresignedURLIsCheckedByTheServer`, against R2.
   A 200 with the right bytes. This is the item the whole plan exists for.
2. **The same read with the signature altered by one character: 403
   `SignatureDoesNotMatch`.** A presigned read that works is not evidence that the
   signature is verified; a refused one is.
3. **The same read with no query string at all: 403.** If the bucket is public, item 1
   proves nothing and the bucket's access policy is the thing to fix first.
4. **One upload, then a `Get` of it, and the reported size equal to the bytes
   written.** This is where the SDK's CRC-64/NVME request checksum meets R2, which is
   the one part of the adapter that has only ever been tested against MinIO's
   implementation of it.
5. **One `Get` of a key that was never written, confirming it surfaces as
   `ErrNotFound`** rather than as a transport error. The adapter maps a typed
   `NoSuchKey`/`NotFound` from the SDK; MinIO returned exactly that typed error, and
   R2's is unverified.
6. **`S3_USE_PATH_STYLE` set deliberately, and recorded as deliberate.** See the
   addressing row above: this is the one configuration value the documentation does
   not settle.
7. **A `Delete` followed by a `Get` returning `ErrNotFound`.**

**Who signs off:** the engineer who merges the environment's `S3_*` values, in the
change that introduces them, with the results of 1-7 pasted into that change. This
repository names no roles, so the sign-off is a named merge rather than a named
person; a checklist with no merge attached to it is not signed off.

## What is explicitly unverified

Stated plainly so that "the MinIO run was green" is not read as "R2 works":

- **No R2 bucket has ever been contacted by this repository.** Every R2 statement
  above is a quotation from Cloudflare's documentation, and none of it has been
  executed.
- **MinIO is a different SigV4 implementation.** That the adapter and MinIO agree
  proves the adapter agrees with *a* server; it is evidence about the AWS SDK and
  about the adapter's own logic, and not about R2's conformance. The `403
  SignatureDoesNotMatch` results in the MinIO run are MinIO's error codes. R2's may
  be the same, and the checklist asserts the code rather than only the status so
  that a difference shows up.
- **The region alias was not exercised.** The MinIO run signed for `us-east-1`; R2
  documents `auto` as the real region with `us-east-1` as an alias. Whether the alias
  behaves identically in the credential scope is untested.
- **Addressing on R2 is untested in both directions**, and the documentation does not
  say which is required.
- **The checksum types R2 supports only as `COMPOSITE` were never sent.** The adapter
  sends CRC-64/NVME, which R2 documents as `FULL_OBJECT`; the other four are
  irrelevant to this adapter and are recorded so nobody later assumes otherwise.
- **Nothing about R2's credential scoping, token lifecycle, custom domains, public
  buckets, CORS, replication, lifecycle rules or CDN** was read or tested. The adapter
  uses none of them.
- **The MinIO run was one machine, one run, plain HTTP, one MinIO release**
  (`RELEASE.2025-09-07T16-13-09Z`). HTTPS takes a different checksum path in the SDK
  and was not exercised at all, on either backend.

## Maintenance

The contract suite is the durable asset: one suite, three backends (a directory, a
double, a real MinIO), and a double that has to stay faithful. It stopped being
faithful twice here, in ways a suite run only against the double could not detect.

If R2 ever needs behaviour MinIO cannot reproduce, extend the double rather than
adding a second suite. A second suite is two contracts that agree until the day
somebody edits one of them.
