# خطای Permission denied در اکستنشن Cursor RTL

## پیام خطا

```text
Cursor RTL: Permission denied. Try running Cursor with elevated privileges or fixing file permissions.
```

## علت مشکل

اکستنشن [Cursor RTL](https://marketplace.visualstudio.com/items?itemName=motcke.cursor-rtl) برای فعال‌سازی RTL، فایل‌های نصب Cursor را روی دیسک پچ می‌کند؛ به‌ویژه:

- `.../resources/app/out/vs/base/parts/sandbox/electron-sandbox/preload.js`
- `.../resources/app/product.json`
- فایل‌های RTL داخل `.../resources/app/out/vs/workbench/`

روی لینوکس مسیر نصب معمولاً این است:

```text
/usr/share/cursor/resources/app
```

اکستنشن برای نوشتن امن از **نوشتن اتمیک** استفاده می‌کند: اول یک فایل موقت داخل **همان پوشه** می‌سازد، بعد آن را روی فایل اصلی `rename` می‌کند.

بنابراین فقط مالکیت خودِ فایل کافی نیست. اگر فایل `preload.js` مال کاربر باشد ولی پوشهٔ والد (`electron-sandbox` یا خودِ `app`) هنوز `root` و غیرقابل‌نوشتن باشد، ساخت فایل موقت با `EACCES` شکست می‌خورد و همان پیام Permission denied نشان داده می‌شود.

این حالت بعد از آپدیت Cursor هم دوباره پیش می‌آید، چون آپدیت معمولاً مالکیت پوشه‌ها را به `root` برمی‌گرداند.

## دستورهای رفع مشکل

در ترمینال:

```bash
sudo chown "$USER:$USER" \
  /usr/share/cursor/resources/app \
  /usr/share/cursor/resources/app/out/vs/base/parts/sandbox/electron-sandbox

sudo chown -R "$USER:$USER" \
  /usr/share/cursor/resources/app/out/vs/base/parts/sandbox/electron-sandbox \
  /usr/share/cursor/resources/app/out/vs/workbench \
  /usr/share/cursor/resources/app/product.json \
  /usr/share/cursor/resources/app/out/main.js
```

### بررسی سریع قبل از Enable

باید برای هر سه مسیر `OK` ببینید:

```bash
python3 - <<'PY'
import os, tempfile
for d in [
  "/usr/share/cursor/resources/app",
  "/usr/share/cursor/resources/app/out/vs/base/parts/sandbox/electron-sandbox",
  "/usr/share/cursor/resources/app/out/vs/workbench",
]:
    try:
        fd, tmp = tempfile.mkstemp(prefix=".rtl-test-", dir=d)
        os.close(fd)
        os.unlink(tmp)
        print("OK", d)
    except Exception as e:
        print("FAIL", d, e)
PY
```

اگر همه `OK` بودند:

1. در Cursor دستور **Enable RTL** یا **Fix After Update** را بزنید.
2. Cursor را کامل Restart کنید.

## نکته

`chown` فقط روی فایل‌ها (مثلاً فقط `main.js`) کافی نیست؛ پوشه‌های والد هم باید برای کاربر قابل نوشتن باشند تا فایل موقت ساخته شود.
