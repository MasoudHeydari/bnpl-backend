# راه‌اندازی HTTPS محلی با mkcert برای dpay.ir

> **محل مستند:** `bnpl-backend/doc/local-development/mkcert/README.md`  
> **کاربرد:** Development و تست محلی BNPL Backend، Envoy Gateway، Frontend و لینک‌های کوتاه بازپرداخت.  
> **دامنهٔ نمونه:** `dpay.ir`، `api.dpay.ir` و `s.dpay.ir`.  
> **محدودهٔ امنیتی:** فقط محیط توسعه؛ این گواهی‌ها و CA محلی نباید در Production استفاده شوند.

این راهنما فرض می‌کند سورس BNPL در مخزن `bnpl-backend` است و پیکربندی Envoy در مخزن جداگانهٔ `platform-infra` نگهداری می‌شود. برای صدور گواهی، **ریشهٔ `bnpl-backend`** را مبنا می‌گیریم؛ تنظیمات Envoy باید مسیر واقعی فایل‌های صادرشده را از طریق Mount یا کپی امن دریافت کنند.

## ۱. معماری و آدرس‌های پیشنهادی

| سرویس | آدرس محلی مرورگر | پاسخ‌دهنده |
| --- | --- | --- |
| Frontend | `https://dpay.ir:5173` | Vite یا Frontend Dev Server |
| BNPL API | `https://api.dpay.ir:8443` | Envoy → BNPL Gin |
| Short Link Resolver | `https://s.dpay.ir:8443/<code>` | Envoy → BNPL Short Link |
| مقصد بازپرداخت | `https://dpay.ir/installments` | مسیر واقعی Frontend، در صورت سرو روی پورت ۴۴۳ |

پورت‌های `5173` و `8443` فقط مثال هستند. نسخهٔ پیشین Envoy Development روی `192.168.1.104:8443` Listen می‌کرد و BNPL را در `192.168.1.104:8080` می‌دید. IPها و Portهای واقعی محیط خودتان را بررسی کنید. `/etc/hosts` فقط DNS را تغییر می‌دهد، نه Listener یا Port را.

**نکتهٔ مهم دربارهٔ مقصد بازپرداخت:** در نمونهٔ Vite روی پورت `5173`، URL بدون پورتِ `https://dpay.ir/installments` خودبه‌خود باز نمی‌شود؛ برای آن باید HTTPS روی پورت `443` در دسترس باشد، یا مقدار `SHORT_LINK_REPAYMENT_DESTINATION_URL` را در محیط Development متناسب با آدرس واقعی Frontend تنظیم کنید (مثلاً `https://dpay.ir:5173/installments`). مقصد باید از سیاست Allowed Hosts عبور کند.

`mkcert` DNS واقعی یا مالکیت `dpay.ir` فراهم نمی‌کند. با Override در `/etc/hosts`، دستگاه شما ممکن است از سایت واقعی این دامنه به محیط محلی هدایت شود. برای جداکردن محیط واقعی از توسعه، نام‌های اختصاصی Development گزینهٔ بهتری هستند؛ این راهنما بنا به نیاز پروژه از `dpay.ir` استفاده می‌کند.

## ۲. نصب mkcert

### Ubuntu / Debian

~~~bash
sudo apt update
sudo apt install -y mkcert libnss3-tools
mkcert -version
~~~

اگر نسخهٔ `mkcert` در مخزن توزیع موجود نبود، از [مستند رسمی نصب](https://github.com/FiloSottile/mkcert#installation) استفاده کنید. `libnss3-tools` برای مدیریت Trust Store مبتنی بر NSS (مثلاً Firefox) لازم است.

### Manjaro / Arch Linux

~~~bash
sudo pacman -Syu mkcert nss
mkcert -version
~~~

### macOS

~~~bash
brew install mkcert nss
mkcert -version
~~~

### Windows

با توجه به Package Manager در PowerShell:

~~~powershell
choco install mkcert
# Or:
scoop bucket add extras
scoop install mkcert
~~~

برای دستورهای بعدی معادل Windows برای مدیریت فایل Hosts و مسیرها را به‌کار ببرید.

## ۳. ساخت و Trust کردن CA محلی

`mkcert -install` را با **کاربر معمولی** اجرا کنید، نه `sudo`:

~~~bash
mkcert -install
mkcert -CAROOT
~~~

ابزار یک CA محلی می‌سازد و Root Certificate آن را به Trust Storeهای قابل‌دسترسی اضافه می‌کند. ممکن است برای مرورگرها به Restart نیاز باشد. در مسیر `mkcert -CAROOT` دو فایل وجود دارد:

- `rootCA.pem`: گواهی عمومی CA، قابل انتقال به دستگاه تست دیگر برای ایجاد اعتماد.
- `rootCA-key.pem`: **کلید خصوصی CA**؛ هرگز آن را کپی، در Git ثبت، در Slack/Telegram ارسال، یا به تیم Frontend تحویل ندهید.

کسی که `rootCA-key.pem` را داشته باشد می‌تواند گواهی‌های مورداعتماد این CA صادر کند. روی دستگاه‌های Production یا دستگاه کاربران نهایی، CA توسعه را نصب نکنید.

## ۴. صدور گواهی برای دامنه‌های dpay.ir

این دستورها را از **ریشهٔ مخزن `bnpl-backend`** اجرا کنید:

~~~bash
cd /path/to/bnpl-backend
mkdir -p doc/local-development/mkcert/certs

mkcert \
  -cert-file doc/local-development/mkcert/certs/dpay.ir.pem \
  -key-file  doc/local-development/mkcert/certs/dpay.ir-key.pem \
  dpay.ir api.dpay.ir s.dpay.ir localhost 127.0.0.1 ::1

chmod 600 doc/local-development/mkcert/certs/dpay.ir-key.pem
~~~

فایل `dpay.ir.pem` گواهی Server و `dpay.ir-key.pem` کلید خصوصی آن است. گواهی فقط برای نام‌های فهرست‌شده در `Subject Alternative Name` معتبر خواهد بود؛ اگر Frontend از `frontend.dpay.ir` استفاده می‌کند، آن نام را نیز هنگام صدور وارد کنید.

ساختار فایل‌های محلی:

~~~text
bnpl-backend/
└── doc/
    └── local-development/
        └── mkcert/
            ├── README.md          # tracked
            ├── .gitignore         # tracked
            └── certs/             # local only; ignored
                ├── dpay.ir.pem
                └── dpay.ir-key.pem
~~~

فایل `.gitignore` همین پوشه `certs/` و برخی فایل‌های حساس گواهی را Ignore می‌کند؛ **وجود .gitignore جایگزین بررسی `git status` قبل از Commit نیست.** کلید خصوصی CA و Server را هرگز در Repository قرار ندهید.

## ۵. تنظیم /etc/hosts

فایل `/etc/hosts` را روی **هر دستگاهی** که مرورگر یا کلاینت HTTPS اجرا می‌کند تغییر دهید؛ این فایل به‌تنهایی DNS کل شبکه را تغییر نمی‌دهد.

### سناریوی A: Envoy و Frontend روی یک دستگاه

اگر هر دو روی `127.0.0.1` یا `0.0.0.0` Bind کرده‌اند:

~~~bash
sudoedit /etc/hosts
~~~

~~~text
127.0.0.1 dpay.ir api.dpay.ir s.dpay.ir
~~~

اگر Envoy فقط روی `192.168.1.104` Bind کرده، نگاشت به `127.0.0.1` کافی نیست؛ باید Listener را مطابق محیط اصلاح کنید یا از IP واقعی استفاده کنید.

### سناریوی B: دستگاه‌های متفاوت در شبکهٔ LAN

فرض کنید Envoy روی `192.168.1.104` و Frontend روی `192.168.1.105` است. روی **هر کلاینت**:

~~~text
192.168.1.104 api.dpay.ir s.dpay.ir
192.168.1.105 dpay.ir
~~~

اگر هر دو روی یک Host هستند، هر سه نام را به IP همان Host نگاشت کنید. مطمئن شوید Frontend روی IP قابل‌دسترسی Listen می‌کند و فایروال پورت `5173` را برای شبکهٔ مجاز باز کرده است.

~~~bash
getent hosts dpay.ir
getent hosts api.dpay.ir
getent hosts s.dpay.ir
~~~

در حضور VPN/Proxy/DoH ممکن است نام‌حل‌کنی مرورگر و مسیر ترافیک با `/etc/hosts` تفاوت داشته باشد. در این حالت Proxy، `NO_PROXY`، DNS و Route واقعی را نیز بررسی کنید. برای دستگاه موبایل، اگر ویرایش Hosts ممکن نیست، از DNS محلی استفاده کنید.

## ۶. TLS در Envoy (پروژهٔ platform-infra)

گواهی mkcert فقط فایل Cert/Key تولید می‌کند. TLS Termination نیازمند تنظیم جداگانه در Envoy است.

**فایل `platform-infra/envoy/generated/development.yaml` توسط `scripts/render-envoy.py` تولید می‌شود؛ خروجی Generated را دستی ویرایش نکنید.** Fragment/Template منبع TLS را پیدا و پس از اصلاح Render کنید.

مثال `DownstreamTlsContext`:

~~~yaml
transport_socket:
  name: envoy.transport_sockets.tls
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
    common_tls_context:
      tls_certificates:
        - certificate_chain:
            filename: /etc/envoy/certs/dpay.ir.pem
          private_key:
            filename: /etc/envoy/certs/dpay.ir-key.pem
~~~

فایل‌های ساخته‌شده در `bnpl-backend` هستند. مسیر Volume روی Host باید از دید Docker Compose صحیح باشد؛ با توجه به محل Compose در `platform-infra`، از مسیر **Absolute** یا مسیر Relative صحیح استفاده کنید. مثال مفهومی:

~~~yaml
services:
  envoy:
    volumes:
      - /absolute/path/to/bnpl-backend/doc/local-development/mkcert/certs:/etc/envoy/certs:ro
~~~

مثال Volume را بدون توجه به Compose موجود کپی نکنید؛ ممکن است سرویس Envoy از قبل Volume گواهی داشته باشد. در آن صورت فقط Source صحیح را جایگزین و نام فایل داخل کانتینر را با `filename` برابر کنید. کلیدها نباید در Image یا Git کپی شوند.

### مرزهای Route و احراز هویت

- Gateway برای APIهای Customer و Merchant Panel باید JWT را بررسی و هدرهای قابل‌اعتماد `X-Auth-*` را پس از حذف نسخهٔ Client تزریق کند.
- `/api/v1/partner/**` توسط Terminal ID/API Key در BNPL احراز هویت می‌شود و نیاز به Gateway JWT ندارد.
- `/api/v1/checkout-sessions/{token}/**` در Hosted Checkout با Token/OTP کار می‌کند؛ نباید JWT Gateway الزامی باشد.
- روت Short Link در Gin روی `GET /:code` و **فقط Host تعریف‌شده در `SHORT_LINK_PUBLIC_HOST`** فعال می‌شود. برای `s.dpay.ir` یک Route/Virtual Host اختصاصی در Envoy در نظر بگیرید، بدون اینکه Host/Authority قبل از رسیدن به BNPL تغییر کند.
- Origin دقیق Frontend در Development، مثلاً `https://dpay.ir:5173`، باید در CORS Gateway باشد؛ Origin شامل پورت است. به خاطر توسعه، CORS/JWT سایر سرویس‌ها را باز نکنید.
- برای Gateway و API حتماً HTTPS برقرار کنید. درخواست Frontend HTTPS به API HTTP ممکن است توسط Browser به‌عنوان Mixed Content مسدود شود.

**ملاحظات HTTPS روی یک پورت:** وجود SANهای `dpay.ir` و `s.dpay.ir` در گواهی کافی است، اما Envoy همچنان باید SNI/TLS و Route بر مبنای Host را درست مدیریت کند. اگر Envoy از گواهی/نام `api.bnpl.test` استفاده می‌کند، مسیرهای جدید را در **منابع تولید کانفیگ** اصلاح کنید.

## ۷. راه‌اندازی Frontend با HTTPS (نمونه Vite)

اگر Frontend با Vite اجرا می‌شود، می‌تواند Server Cert/Key صادرشده را بخواند. کلید خصوصی را در متغیرهای `VITE_*` قرار ندهید؛ این متغیرها ممکن است در Bundle مرورگر ظاهر شوند.

نمونه `vite.config.ts`:

~~~typescript
import { defineConfig } from 'vite';
import { readFileSync } from 'node:fs';

const cert = process.env.LOCAL_TLS_CERT;
const key = process.env.LOCAL_TLS_KEY;

if (!cert || !key) {
  throw new Error('LOCAL_TLS_CERT and LOCAL_TLS_KEY are required');
}

export default defineConfig({
  server: {
    host: '0.0.0.0',
    port: 5173,
    strictPort: true,
    allowedHosts: ['dpay.ir'],
    https: {
      cert: readFileSync(cert),
      key: readFileSync(key),
    },
  },
});
~~~

در Shell پروژه Frontend:

~~~bash
export LOCAL_TLS_CERT="/absolute/path/to/bnpl-backend/doc/local-development/mkcert/certs/dpay.ir.pem"
export LOCAL_TLS_KEY="/absolute/path/to/bnpl-backend/doc/local-development/mkcert/certs/dpay.ir-key.pem"
npm run dev
~~~

مرورگر را روی `https://dpay.ir:5173` باز کنید.

اگر Frontend روی کامپیوتر دیگری اجرا می‌شود، آن Host برای سرو کردن HTTPS به Server Certificate و Private Key نیاز دارد؛ انتقال آن‌ها باید با روش امن انجام شود. برای کاهش توزیع کلید خصوصی، می‌توان روی همان Host گواهی مجزایی با CA توسعه صادر کرد یا TLS را روی Proxy کنترل‌شده خاتمه داد.

Frontend هنگام درخواست به BNPL از `https://api.dpay.ir:8443/api/v1` استفاده کند. برای APIهای احرازشده Identity/Gateway Token بفرستد، اما `X-Auth-*` را خودش نسازد. برای Hosted Checkout مبتنی بر Token/OTP، نه JWT پنل و نه `X-API-Key` Partner لازم نیست.

برای Next.js، webpack-dev-server و سایر ابزارها نحوهٔ افزودن TLS فرق می‌کند؛ گواهی موجود را با پیکربندی صحیح همان ابزار ارائه دهید.

## ۸. Short Link و پیامک یادآوری بازپرداخت

مقادیر پیشنهادی فعلی:

~~~dotenv
SHORT_LINK_ALLOWED_DESTINATION_HOST=dpay.ir
SHORT_LINK_PUBLIC_HOST=s.dpay.ir
SHORT_LINK_REPAYMENT_DESTINATION_URL=https://dpay.ir/installments
~~~

- `ALLOWED_DESTINATION_HOST` فقط Host مجاز مقصد است؛ Scheme و Path ندارد.
- `PUBLIC_HOST` فقط نام دامنهٔ Short Link است؛ بدون `https://` یا Path.
- `REPAYMENT_DESTINATION_URL` آدرس کامل HTTPS صفحهٔ بازپرداخت است و باید واقعاً باز شود.

کد BNPL لینک کوتاه یادآوری را به صورت `s.dpay.ir/<code>` تولید می‌کند؛ عمداً Scheme و Port ندارد. بنابراین برای اینکه کلیک روی SMS بدون افزودن پورت کار کند، باید `s.dpay.ir` روی پورت HTTPS پیش‌فرض **443** در دسترس باشد. Listen کردن Envoy فقط روی `8443` کافی نیست؛ از Front Proxy یا Port Forward یا Listener مناسب استفاده کنید.

در آزمون دستی روی `8443` می‌توانید `https://s.dpay.ir:8443/REAL_CODE` را استفاده کنید، اما Code باید در دیتابیس وجود داشته باشد. پاسخ موفق Resolver، HTTP `302` با `Location` مقصد است.

**توجه:** در پیاده‌سازی فعلی، همهٔ لینک‌های یادآوری بازپرداخت به یک Destination URL ثابت اشاره می‌کنند؛ نه به URL شخصی یک قسط. Frontend مقصد بعد از Login باید اقساط مربوط به کاربر را از BNPL دریافت کند.

## ۹. تست DNS، گواهی و Endpointها

~~~bash
# Inspect certificate SANs
openssl x509 \
  -in doc/local-development/mkcert/certs/dpay.ir.pem \
  -noout -ext subjectAltName

# Vite HTTPS
curl -v https://dpay.ir:5173/

# BNPL liveness (GET, not HEAD)
curl -v https://api.dpay.ir:8443/healthz/live

# Bypass hosts mapping for one request, retaining Host and SNI
curl -v --resolve api.dpay.ir:8443:192.168.1.104 \
  https://api.dpay.ir:8443/healthz/live

# TLS/SNI inspection
openssl s_client \
  -connect 192.168.1.104:8443 \
  -servername api.dpay.ir \
  -showcerts </dev/null

# A made-up code should return 404; a real one should return 302
curl -v https://s.dpay.ir:8443/invalidcode
curl -sS -D - -o /dev/null https://s.dpay.ir:8443/REAL_CODE
~~~

آدرس‌های IP در تست بالا فقط مثال‌اند. `curl` بدون `-k` باید Trust گواهی را تأیید کند. `curl -k` یا غیرفعال‌کردن Certificate Verification را راهکار دائمی قرار ندهید.

برای Resolver از `GET` استفاده کنید؛ `curl -I` درخواست `HEAD` می‌فرستد، در حالی که Route ثبت‌شدهٔ Gin فقط `GET` است.

اگر TLS درست شد اما HTTP 401/404 دیدید، مشکل از JWT/Route/Host یا آماده نبودن Backend است، نه لزوماً گواهی.

## ۱۰. دسترسی از سیستم دیگر و Trust مرورگر

برای هر دستگاه دوم:

1. نام دامنه را در Hosts یا DNS داخلی به IP واقعی Frontend و Envoy Resolve کنید.
2. **فقط `rootCA.pem` عمومی** را به روش امن به آن دستگاه انتقال داده و به Store موردنیاز اضافه کنید؛ **هرگز `rootCA-key.pem` را منتقل نکنید**.

برای Ubuntu Client:

~~~bash
sudo cp rootCA.pem /usr/local/share/ca-certificates/dpay-local-dev.crt
sudo update-ca-certificates
~~~

Firefox ممکن است Store مجزای NSS داشته باشد؛ CA را در Certificate Authorities مربوط به مرورگر Import کنید و آن را Restart کنید. iOS بعد از نصب گواهی CA ممکن است فعال‌سازی Full Trust بخواهد. Android برای اپ Native ممکن است Network Security Config توسعه را لازم داشته باشد.

اعتماد به CA فقط برای **کلاینت** کافی است؛ **سروری** که HTTPS ارائه می‌دهد، باید به Server Cert و Private Key معتبر خودش دسترسی داشته باشد. CA توسعه را روی دستگاه کاربران نهایی نصب نکنید.

در تست‌های Node.js که Trust Store متفاوت دارند، در صورت نیاز:

~~~bash
export NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem"
~~~

این متغیر را پیش از Start کردن Process Node تنظیم کنید؛ گواهی ریشه را داخل Bundle عمومی Frontend قرار ندهید.

## ۱۱. عیب‌یابی

| علامت | علت محتمل | بررسی |
| --- | --- | --- |
| `ERR_CERT_AUTHORITY_INVALID` | CA در دستگاه/مرورگر Trust نشده | `mkcert -install`، Trust Store مرورگر، Restart |
| `ERR_CERT_COMMON_NAME_INVALID` | نام دامنه در SAN نیست | ساخت گواهی جدید با Host درست |
| `ECONNREFUSED` | Hosts، Listener، پورت یا فایروال اشتباه | `getent hosts`، `ss -lntp`، Docker ports |
| CORS/Preflight Failed | Origin یا Header درست مجاز نشده | Origin دقیق `https://dpay.ir:5173` در Gateway Dev |
| Hosted Checkout HTTP 401 | استثنای JWT برای Hosted Token Routeها وجود ندارد | ترتیب و Match قوانین `jwt_authn` |
| Short Link HTTP 404 | Host، Route یا Code نامعتبر است | Virtual Host، `:authority`، DB و `SHORT_LINK_PUBLIC_HOST` |
| BNPL Health HTTP 404 | Rewrite یا Health Gin فعال نیست | `/healthz/live`، تنظیم Runtime |
| Mixed Content | Frontend HTTPS و Backend HTTP | استفاده از API Base URL دارای HTTPS |
| HMR قطع می‌شود | WS/Host/Port/TLS در Vite نادرست است | تنظیم HMR و مسیر شبکه |
| لینک SMS باز نمی‌شود | `s.dpay.ir` فقط روی 8443 کار می‌کند | HTTPS روی پورت 443 فراهم شود |

## ۱۲. چک‌لیست قبل از تست یکپارچه

- [ ] `mkcert` نصب شده و `mkcert -install` با کاربر عادی اجرا شده است.
- [ ] گواهی شامل `dpay.ir`، `api.dpay.ir` و `s.dpay.ir` است.
- [ ] Cert/Key در `doc/local-development/mkcert/certs/` و خارج از Git هستند.
- [ ] `/etc/hosts` روی هر کلاینت به IP واقعی اشاره می‌کند.
- [ ] Envoy گواهی مناسب را Mount کرده و کانفیگ از **Source** دوباره Render شده است.
- [ ] Route اختصاصی Short Link با API و سایر سرویس‌ها تداخل ندارد.
- [ ] JWT APIهای محافظت‌شده فعال است و Partner/Hosted فقط استثنای مخصوص خودشان را دارند.
- [ ] Frontend روی HTTPS است و CORS برای Origin واقعی Dev مجاز شده است.
- [ ] `curl` بدون `-k` گواهی را معتبر می‌داند.
- [ ] اگر SMS بدون Port استفاده می‌شود، `s.dpay.ir` روی 443 پاسخ می‌دهد.
- [ ] هیچ CA/Server Private Key یا گواهی توسعه‌ای در Production کپی نشده است.

## منابع رسمی

- [mkcert / Installation](https://github.com/FiloSottile/mkcert#installation)
- [Vite: server.https](https://vite.dev/config/server-options#server-https)
- [Envoy: Downstream TLS](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/tls.proto)

> **نگه‌داری:** این راهنما در مخزن `bnpl-backend` قرار دارد. در صورت انتقال مستند به `platform-infra`، مسیرهای صدور گواهی، Volumeها و مثال‌های `LOCAL_TLS_*` را با محل جدید هماهنگ کنید.
