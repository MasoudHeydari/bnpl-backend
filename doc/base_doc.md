## ۱. پایه سیستم و کانفیگ‌ها

### ۱.۱. Customer Service (مدیریت مشتری)

- سرویس بکند:
    
    - مدل/سرویس `Customer`
    - ای پی ای API برای ساختن و خواندن مشتری
        
- عملکرد:
    
    - نگه داشتن اطلاعات پایه مشتری (id، موبایل، کدملی، status)
    - این همون چیزی‌ه که همهٔ planها، orderها و limitها بهش وصل می‌شن.
        

---

### ۱.۲. Merchant Service + Tax Profile (مدیریت تأمین‌کننده + پروفایل مالیاتی)

- سرویس بکند:
    
    - مدل/سرویس `Merchant`
    - فیلدهای مالیاتی و تسویه (tax_id، tax_regime، tax_rate، price_includes_tax، settlement_terms)
        
    - ای پی ای API برای ساخت/ویرایش/خواندن merchant
        
- عملکرد:
    
    - نگه داشتن اطلاعات حقوقی و مالیاتی هر تأمین‌کننده
    - نگه داشتن تنظیمات BNPL برای هر merchant (مثلاً کارمزد، شرایط تسویه)
    - تعیین این‌که قیمت‌های merchant شامل مالیات هستند یا نه.
        

---

### ۱.۳. Finance & Tax Config Service (کانفیگ مالی/مالیاتی کلی سیستم)

- بکند باید بسازه:
    
    - سرویس/جدول برای نگه داشتن نرخ‌های مالیات عمومی، نوع مالیات (VAT و …)، تنظیمات پیش‌فرض
        
- عملکرد:
    
    - مرکزی برای این‌که سیستم بداند چه نوع مالیاتی داریم و نرخ‌ها چنده
    - استفاده در محاسبه مبلغ‌ها در order، plan، settlement.
        

---

## ۲. سفارش و قیمت‌گذاری (با مالیات)

### ۲.۱. Order Intake & Storage (دریافت و ذخیره سفارش)

- سرویس بکند:
    
    - مدل/سرویس `Order`
    - ای پی ای API که از سمت merchant صدا زده می‌شه (create/update order)
        
- عملکرد:
    
    - گرفتن سفارش از merchant (id، customer، merchant، مبلغ‌ها)
    - ذخیرهٔ وضعیت سفارش تا بعداً رویش plan BNPL ساخته بشه.
        

---

### ۲.۲. Tax-Aware Pricing Engine (موتور محاسبه مبلغ + مالیات)

- سرویس بکند:
    
    - سرویس محاسبه مبلغ که این‌ها رو تولید/نگه داره:
        
        - مبلغ خالص کالاها (net)
        - مالیات کالاها (tax)
        - مبلغ نهایی با مالیات (gross)
            
- عملکرد:
    
    - بر اساس تنظیمات merchant (شامل مالیات/بدون مالیات) و کانفیگ مالیاتی،  
        مبلغ سفارش را به `net / tax / gross` بشکند
        
    - این سه‌تا عدد، پایهٔ همه چیز بعدی می‌شوند (plan، settle، exposure).
        

---

### ۲.۳. BNPL Fee Calculation (محاسبه کارمزد BNPL و مالیاتش)

> حتی اگر اول کار کارمزدی نگیریم باید در معماری تعریف بشه.

- سرویس بکند:
    
    - سرویس `BnplFeeCalculator`
        
    - ارتباط آن با merchant config (fee type, fee rate, fee tax rate)
        
- عملکرد:
    
    - محاسبهٔ کارمزد BNPL برای هر order (روی net یا gross)
    - تفکیک کارمزد به: `fee_net`, `fee_tax`, `fee_gross`
    - این عدد در settlement با merchant و گزارش مالیاتی استفاده می‌شه.
        

---

## ۳. BNPL Plan & Installments (قرارداد و اقساط)

### ۳.۱. Plan Management Service (مدیریت قرارداد BNPL)

- سرویس بکند:
    
    - مدل/سرویس `Plan` (contract)
        
    - ای پی ای API برای ایجاد plan روی یک order
        
- عملکرد:
    
    - تبدیل یک order تأییدشده به قرارداد BNPL
    - نگه داشتن مبلغ‌های اصلی plan:
        - اصل کالا: net / tax / gross
        - کارمزد BNPL از مشتری (اگر هست): net / tax / gross
        - مجموع قابل پرداخت مشتری
        - (تعهد ما به عنوان ضامن) exposure_amount .
            
    - وضعیت plan (PENDING / ACTIVE / COMPLETED / CANCELLED / DEFAULTED).
        

---

### ۳.۲. Installment Schedule Generator (تولید برنامه اقساط با مالیات)

- سرویس بکند:
    
    - سرویس تولید اقساط
        
    - مدل `Installment`
        
- عملکرد:
    
    - گرفتن `total_payable_amount` plan و تعداد اقساط
    - تقسیم مبلغ و مالیات بین اقساط (total_net / total_tax / total_gross برای هر قسط)
    - تولید اقساط با تاریخ سررسید و وضعیت اولیه DUE.
        

---

### ۳.۳. Plan & Installment State Machine (ماشین وضعیت قرارداد و اقساط)

- سرویس بکند:
    
    - سرویس تغییر وضعیت (state machine)
        
    - ای پی ای API برای:
        
        -  فعال activate_plan
            
        - لغو cancel_plan
            
        - معوق mark_installment_paid / overdue
            
        - نکول شده mark_plan_completed / defaulted
            
- عملکرد:
    
    - کنترل تمام تغییر وضعیت‌های plan و اقساط
    - به‌روزرسانی exposure و limit بر اساس این وضعیت‌ها.
        

---

## ۴. Credit & Risk (سقف اعتباری و قوانین ساده)

### ۴.۱. Customer Limit Service (مدیریت سقف اعتباری)

- سرویس بکند:
    
    - مدل/سرویس `CustomerLimit`
        
    - ای پی ای API برای خواندن/آپدیت limit
        
- عملکرد:
    
    - نگه داشتن `credit_limit`, `used_limit`, `available_limit` برای هر مشتری
    - قبل از ایجاد plan، چک کند آیا `exposure_amount` جدید داخل limit هست یا نه
    - با active / cancel / default شدن plan، `used_limit` را به‌روزرسانی کند.
        

---

### ۴.۲. Simple Risk Rule Engine (موتور قوانین ساده ریسک)

- سرویس بکند:
    
    - سرویس rule engine ساده
        
    - یک جدول/کانفیگ برای ruleها
        
- عملکرد:
    
    - اعمال چند قانون پایه مثل:
        - حداقل / حداکثر مبلغ order
        - ممنوعیت برای مشتریان با overdue فعال
        - لیست سیاه مشتری/merchant
            
    - خروجی: `APPROVE`, `REJECT`, یا در آینده `REVIEW`.
        

---

## ۵. Payments (پرداخت اقساط)

### ۵.۱. Installment Payment API (ثبت پرداخت قسط)

- سرویس بکند:
    
    - ای پی ای API برای شروع پرداخت قسط (create payment request)
        
    - ای پی ای API/callback برای دریافت نتیجهٔ پرداخت (webhook)
        
- عملکرد:
    
    - گرفتن درخواست پرداخت برای یک قسط (یا چند قسط)
        
    - ارسال request به payment gateway
        
    - دریافت نتیجه و ثبت payment
        
    - وصل کردن پرداخت به installment و آپدیت وضعیت و مبلغ‌های پرداخت‌شده.
        

---

### ۵.۲. Payment Gateway Integration (اتصال به درگاه پرداخت)

- سرویس بکند:
    
    - ماژول/سرویس برای ارتباط با حداقل یک درگاه پرداخت
        
- عملکرد:
    
    - ساخت لینک/توکن پرداخت
        
    - دریافت callback / webhook
        
    - هندل سناریوهای SUCCESS / FAILED / PENDING
        
    - بازگرداندن نتیجه به سرویس اقساط.
        

---

### ۵.۳. Payment Reconciliation Job (بررسی و تصحیح پرداخت‌ها – حداقلی)

- سرویس بکند:
    
    - یک job دوره‌ای (cron)
        
- عملکرد:
    
    - چک کردن پرداخت‌هایی که statusشان نامشخص مانده (مثلاً PENDING)
        
    - سینک sync دوباره با درگاه و اصلاح وضعیت‌ها تا قسط‌ها درست علامت بخورند.
        

---

## ۶. Settlements & Guarantee Exposure (تسویه با تأمین‌کننده و تعهد ضامن)

### ۶.۱. Merchant Settlement Calculator (ماشین محاسبه تسویه با مالیات)

- سرویس بکند:
    
    - سرویس `SettlementCalculator`
        
    - مدل `MerchantSettlement`
        
- عملکرد:
    
    - برای هر order/plan حساب کند:
        
        - مبلغ فاکتور کالا برای merchant: net / tax / gross
            
        - کارمزد BNPL از merchant: net / tax / gross
            
        - مبلغ خالص قابل پرداخت به merchant
            
    - ثبت یک رکورد settlement با این اعداد.
        

---

### ۶.۲. Settlement Execution / Export (اجرای تسویه یا خروجی بانکی)

- سرویس بکند:
    
    - سرویس ساخت batch settlement (در صورت تسویه گروهی)
        
    - ای پی ای API یا فایل‌ساز برای خروجی به سیستم پرداخت/بانک
        
- عملکرد:
    
    - جمع‌کردن settlementهای آماده پرداخت
        
    - تولید خروجی قابل ارسال به بانک یا سیستم مالی
        
    - ذخیره وضعیت هر settlement (PENDING / SENT / PAID / FAILED).
        

---

### ۶.۳. Exposure Tracking Service (ردگیری تعهد ضامن)

- سرویس بکند:
    
    - سرویس یا جدول `Exposure` (می‌تونه روی plan باشد یا جدا)
        
- عملکرد:
    
    - نگه داشتن این‌که سیستم به عنوان ضامن، چه مقدار تعهد جاری دارد
        
    - استفاده برای گزارش داخلی و ریسک (کل exposure روی مشتری، روی merchant، روی محصول).
        

---

## ۷. Jobs & Notifications پایه

### ۷.۱. Due/Overdue Scheduler (jobِ علامت‌گذاری سررسید و تأخیر)

- سرویس بکند:
    
    - عملیات job دوره‌ای (روزانه) برای اقساط
        
- عملکرد:
    
    - پیدا کردن اقساطی که تاریخشان گذشته و هنوز پرداخت نشده‌اند
        
    - تغییر status آن‌ها به OVERDUE
        
    - در صورت گذشت n روز، mark کردن plan به DEFAULTED (طبق قانون شما).
        

---

### ۷.۲. Notification Trigger Service (تریگر نوتیفیکیشن)

> خود ارسال SMS/ایمیل می‌تونه سرویس جدا یا external باشه؛ هسته فقط event تولید کنه.

- سرویس بکند:
    
    - سرویس تولید event یا queue برای نوتیفیکیشن
        
- عملکرد:
    
    - تولید event در لحظه‌هایی مثل:
        
        - ایجاد طرح plan
        - چند روز قبل از سررسید قسط
        - روز سررسید
        - معوقه overdue شدن
            
    - بقیهٔ سیستم‌ها می‌تونن این event رو مصرف کنند و پیام بفرستند.
        

---

## ۸. Admin & Audit (حداقل لازم برای کار عملیاتی)

### ۸.۱. Admin Query APIs (APIهای ادمینی برای دیدن اطلاعات)

- سرویس بکند:
    
    - خروجی endpointهایی برای لیست/جستجوی:
        
        - **مشتریان** customers
            
        - **پذیرندگان / فروشندگان** merchants
            
        - **سفارش‌ها** orders
            
        - **طرح‌ها / برنامه‌ها** plans
            
        - **اقساط** installments
            
        - **تسویه‌حساب‌ها** settlements
            
- عملکرد:
    
    - امکان بررسی وضعیت مشتری، قسط‌ها و تسویه‌ها برای تیم داخلی و پنل ادمین.
        

---

### ۸.۲. Audit Log Service (ثبت تغییرات مهم)

- سرویس بکند:
    
    - سرویس/جدول `AuditLog`
        
- عملکرد:
    
    - ثبت هر تغییر حساس:
        
        - تغییر limit مشتری
            
        - تغییر دستی وضعیت قسط یا plan
            
        - اجرای settlement
            
    - کمک برای بررسی بعدی و نیازهای قانونی/مالیاتی.
        

---

### ۸.۳. Basic Role/Permission Hook (حداقلی)

- سرویس بکند:
    
    - نقاط کنترل برای دسترسی (integration با سیستم auth/role)
        
- عملکرد:
    
    - محدود کردن APIهای حساس به نقش‌های خاص (مثلاً فقط Ops بتواند قسط را دستی paid کند).