package text

// catalog holds every text, by key. Domain words follow the Malay in GLOSSARY.md; product names stay English.
var catalog = map[string]entry{
	// Layout: header
	"header.logo-alt":     {en: "CAVELMS, your digital learning hub", ms: "CAVELMS, hab pembelajaran digital anda"},
	"header.main-nav":     {en: "Main", ms: "Utama"},
	"header.menu":         {en: "Menu", ms: "Menu"},
	"header.account":      {en: "Account", ms: "Akaun"},
	"header.logged-in-as": {en: "Logged in as %s", ms: "Log masuk sebagai %s"},
	"header.admin-area":   {en: "Admin area", ms: "Ruang Pentadbir"},
	"header.log-out":      {en: "Log out", ms: "Log keluar"},
	"header.log-in":       {en: "Log in", ms: "Log masuk"},
	"header.register":     {en: "Register", ms: "Daftar"},
	"header.language":     {en: "Language", ms: "Bahasa"},
	"nav.courses":         {en: "Courses", ms: "Kursus"},
	"nav.lab":             {en: "Lab", ms: "Lab"},
	"nav.gallery":         {en: "Gallery", ms: "Galeri"},
	"nav.cavetext":        {en: "CaveText", ms: "CaveText"},
	"nav.check-cert":      {en: "Check a certificate", ms: "Semak sijil"},
	"nav.my-courses":      {en: "My courses", ms: "Kursus saya"},

	// Layout: footer
	"footer.powered-by":      {en: "Powered by The Cave Consult (TCC)", ms: "Dikuasakan oleh The Cave Consult (TCC)"},
	"footer.learn":           {en: "Learn", ms: "Belajar"},
	"footer.all-courses":     {en: "All courses", ms: "Semua kursus"},
	"footer.my-certificates": {en: "My certificates", ms: "Sijil saya"},
	"footer.reset-password":  {en: "Reset password", ms: "Set semula kata laluan"},
	"footer.contact":         {en: "Contact", ms: "Hubungi"},

	// ---- Privacy notice (/privacy) and the Register consent line: begin ----
	// The facts are recorded in docs/adr/0006. Change the date in privacy.updated whenever the notice changes.
	"privacy.link":     {en: "Privacy Notice", ms: "Notis Privasi"},
	"privacy.updated":  {en: "Last updated 22 October 2026", ms: "Kemas kini terakhir 22 Oktober 2026"},
	"register.consent": {en: "By registering you confirm you are 18 or older and agree to the", ms: "Dengan mendaftar, anda mengesahkan bahawa anda berumur 18 tahun ke atas dan bersetuju dengan"},
	"privacy.intro": {
		en: "This notice explains what personal data CAVELMS and CaveText accounts hold, why, where it goes and what you can do about it, as the Personal Data Protection Act 2010 (PDPA) requires.",
		ms: "Notis ini menerangkan data peribadi yang disimpan oleh akaun CAVELMS dan CaveText, sebabnya, ke mana data itu pergi dan apa yang boleh anda lakukan mengenainya, seperti yang dikehendaki oleh Akta Perlindungan Data Peribadi 2010 (PDPA).",
	},
	"privacy.prevails": {
		en: "This notice is in Malay and English. If the two differ, the English version prevails.",
		ms: "Notis ini ada dalam bahasa Melayu dan bahasa Inggeris. Jika kedua-duanya berbeza, versi bahasa Inggeris terpakai.",
	},
	"privacy.who.title": {en: "Who looks after your data", ms: "Siapa yang menjaga data anda"},
	"privacy.who.p": {
		en: "The Cave Consult (TCC), which runs CAVELMS, is the data user: it decides what data is collected and why. Wasabie Tech builds and hosts the site for The Cave Consult and is its data processor: it handles the data only to run the site, on The Cave Consult's instructions.",
		ms: "The Cave Consult (TCC), yang mengendalikan CAVELMS, ialah pengguna data: ia menentukan data yang dikumpul dan sebabnya. Wasabie Tech membina dan mengehos laman ini untuk The Cave Consult dan ialah pemproses datanya: ia mengendalikan data hanya untuk menjalankan laman ini, atas arahan The Cave Consult.",
	},
	"privacy.collect.title": {en: "What we collect and why", ms: "Data yang kami kumpul dan sebabnya"},
	"privacy.collect.p": {
		en: "All of it comes from you, as you register and use the site. Your name, email address and a password are needed to register; without them we cannot give you an account.",
		ms: "Semuanya datang daripada anda, semasa anda mendaftar dan menggunakan laman ini. Nama, alamat e-mel dan kata laluan diperlukan untuk mendaftar; tanpanya kami tidak dapat memberi anda akaun.",
	},
	"privacy.collect.account": {
		en: "Your account: your name, email address, password (kept only as a one-way hash, never as you typed it) and whether you have verified your email. We use them to log you in, to put your name on your Certificates, and to send you the verification and password reset emails.",
		ms: "Akaun anda: nama, alamat e-mel, kata laluan (disimpan sebagai hash sehala sahaja, bukan seperti yang anda taip) dan sama ada anda telah mengesahkan e-mel anda. Kami menggunakannya untuk membolehkan anda log masuk, meletakkan nama anda pada Sijil anda, dan menghantar e-mel pengesahan dan set semula kata laluan kepada anda.",
	},
	"privacy.collect.sessions": {
		en: "Your logins: a cookie in your browser and a matching record on the server keep you logged in until you log out or the login expires.",
		ms: "Log masuk anda: satu kuki dalam pelayar anda dan rekod yang sepadan di pelayan mengekalkan log masuk anda sehingga anda log keluar atau log masuk itu tamat tempoh.",
	},
	"privacy.collect.progress": {
		en: "Your Course progress: the Courses you join, the Lessons you complete, and your Exam attempts and scores, so that you can carry on where you left off and earn Certificates.",
		ms: "Kemajuan Kursus anda: Kursus yang anda sertai, Pelajaran yang anda selesaikan, serta percubaan dan markah Peperiksaan anda, supaya anda boleh menyambung dari tempat anda berhenti dan memperoleh Sijil.",
	},
	"privacy.collect.certificates": {
		en: "Your Certificates: your name, the Course and the date. Anyone who has a Certificate's ID can check it on this site and will see your name on it.",
		ms: "Sijil anda: nama anda, Kursus dan tarikh. Sesiapa yang mempunyai ID sesuatu Sijil boleh menyemaknya di laman ini dan akan melihat nama anda padanya.",
	},
	"privacy.collect.lab": {
		en: "Your Lab work: your Dashboards, the Datasets you upload to them and your Deployments. A Deployment's link is public: anyone who has it can open the Dashboard. The Lab is for the Samples it provides, so do not upload anyone else's personal data to it.",
		ms: "Kerja Lab anda: Dashboard anda, Set data yang anda muat naik kepadanya dan Deployment anda. Pautan sesuatu Deployment terbuka kepada umum: sesiapa yang memilikinya boleh membuka Dashboard itu. Lab adalah untuk Sampel yang disediakannya, jadi jangan muat naik data peribadi orang lain ke dalamnya.",
	},
	"privacy.collect.plans": {
		en: "Your Plans: which Lab Plan you have and when each Plan period starts and ends, so that the Lab is open to you while it runs.",
		ms: "Pelan anda: Pelan Lab yang anda miliki dan bila setiap Tempoh Pelan bermula dan berakhir, supaya Lab dibuka untuk anda sepanjang tempoh itu.",
	},
	"privacy.collect.cavetext": {
		en: "Your CaveText account: its place (where you came in the order of first CaveText sign-ins), your Snippets, your User master prompt, your CaveText Pro periods, and the sign-in tokens that keep the CaveText extension signed in, so that CaveText works the same in every browser you sign in to.",
		ms: "Akaun CaveText anda: tempatnya (kedudukan anda dalam urutan log masuk pertama CaveText), Snippet anda, User master prompt anda, tempoh CaveText Pro anda, dan token log masuk yang mengekalkan sambungan CaveText dalam keadaan log masuk, supaya CaveText berfungsi sama dalam setiap pelayar yang anda gunakan untuk log masuk.",
	},
	"privacy.collect.language": {
		en: "Your language: the language you pick, saved on your account and in a cookie, so that the site stays in it.",
		ms: "Bahasa anda: bahasa yang anda pilih, disimpan pada akaun anda dan dalam kuki, supaya laman ini kekal dalam bahasa itu.",
	},
	"privacy.collect.log": {
		en: "The System log: errors the site runs into, with the kind of request and the page address, kept 30 days so that faults can be found and fixed.",
		ms: "Log sistem: ralat yang berlaku pada laman ini, bersama jenis permintaan dan alamat halaman, disimpan 30 hari supaya kerosakan dapat dikesan dan dibaiki.",
	},
	"privacy.see.title": {en: "Who can see it", ms: "Siapa yang boleh melihatnya"},
	"privacy.see.admins": {
		en: "Admins, the people who run the site, can see Learners' names, email addresses, Course progress, Certificates and Deployments, and whether they have a Plan or CaveText Pro, so that they can run Courses, issue Certificates and help Learners.",
		ms: "Pentadbir, iaitu orang yang mengendalikan laman ini, boleh melihat nama, alamat e-mel, kemajuan Kursus, Sijil dan Deployment Pelajar, serta sama ada mereka mempunyai Pelan atau CaveText Pro, supaya mereka dapat mengendalikan Kursus, mengeluarkan Sijil dan membantu Pelajar.",
	},
	"privacy.see.private": {
		en: "No one reads your Snippets or your User master prompt, unless you ask us for help with them, it is needed to keep the service secure, or the law requires it. Admin screens show only how many Snippets a Learner has, never what they say.",
		ms: "Tiada sesiapa membaca Snippet atau User master prompt anda, kecuali jika anda meminta bantuan kami mengenainya, ia perlu untuk memastikan perkhidmatan ini selamat, atau undang-undang menghendakinya. Skrin Pentadbir hanya menunjukkan bilangan Snippet seseorang Pelajar, bukan isinya.",
	},
	"privacy.see.others": {
		en: "We do not sell your data or share it with anyone else, apart from the service providers below that keep the site running, or when the law requires it.",
		ms: "Kami tidak menjual data anda atau berkongsinya dengan sesiapa, selain penyedia perkhidmatan di bawah yang memastikan laman ini berjalan, atau apabila undang-undang menghendakinya.",
	},
	"privacy.where.title": {en: "Where it is stored", ms: "Di mana ia disimpan"},
	"privacy.where.p1": {
		en: "The site runs on a server in France, rented from Contabo. Every night the data is backed up, encrypted, to that server and to Cloudflare R2, which may keep it in the United States. Verification and password reset emails are sent through Resend, in the United States, which sees your name and email address as it delivers them.",
		ms: "Laman ini berjalan pada pelayan di Perancis, yang disewa daripada Contabo. Setiap malam data disandarkan, dalam bentuk tersulit, ke pelayan itu dan ke Cloudflare R2, yang mungkin menyimpannya di Amerika Syarikat. E-mel pengesahan dan set semula kata laluan dihantar melalui Resend, di Amerika Syarikat, yang melihat nama dan alamat e-mel anda semasa menghantarnya.",
	},
	"privacy.where.p2": {
		en: "So your data leaves Malaysia. It goes to these providers only so that the site can run, can be restored after a fault, and can send you emails.",
		ms: "Oleh itu data anda keluar dari Malaysia. Ia dihantar kepada penyedia ini hanya supaya laman ini dapat berjalan, dapat dipulihkan selepas kerosakan, dan dapat menghantar e-mel kepada anda.",
	},
	"privacy.keep.title": {en: "How long we keep it", ms: "Berapa lama kami menyimpannya"},
	"privacy.keep.p": {
		en: "We keep your data until you ask us to delete your account. An account that is not used is not deleted on its own. After a deletion, copies stay in the backups for up to 6 months, until the backups that hold them expire. The System log keeps each entry for 30 days.",
		ms: "Kami menyimpan data anda sehingga anda meminta kami memadam akaun anda. Akaun yang tidak digunakan tidak dipadam dengan sendirinya. Selepas pemadaman, salinannya kekal dalam sandaran sehingga 6 bulan, sehingga sandaran yang menyimpannya tamat tempoh. Log sistem menyimpan setiap catatan selama 30 hari.",
	},
	"privacy.pay.title": {en: "Payments", ms: "Pembayaran"},
	"privacy.pay.p": {
		en: "CaveText Pro and Plans are paid through the payment provider, or arranged directly with The Cave Consult. CAVELMS never sees your card details.",
		ms: "CaveText Pro dan Pelan dibayar melalui penyedia pembayaran, atau diuruskan terus dengan The Cave Consult. CAVELMS tidak pernah melihat butiran kad anda.",
	},
	"privacy.cookies.title": {en: "Cookies and email", ms: "Kuki dan e-mel"},
	"privacy.cookies.p": {
		en: "CAVELMS sets only two cookies: one that keeps you logged in and one that remembers your language. There are no analytics, advertising or tracking cookies. We send only the emails your account needs, such as email verification and password reset, and no marketing email.",
		ms: "CAVELMS hanya menetapkan dua kuki: satu untuk mengekalkan log masuk anda dan satu untuk mengingati bahasa anda. Tiada kuki analitik, pengiklanan atau penjejakan. Kami hanya menghantar e-mel yang diperlukan oleh akaun anda, seperti pengesahan e-mel dan set semula kata laluan, dan tiada e-mel pemasaran.",
	},
	"privacy.adults.title": {en: "For adults only", ms: "Untuk orang dewasa sahaja"},
	"privacy.adults.p": {
		en: "CAVELMS is for people aged 18 or older. If you are under 18, please do not register.",
		ms: "CAVELMS untuk mereka yang berumur 18 tahun ke atas. Jika anda berumur di bawah 18 tahun, sila jangan mendaftar.",
	},
	"privacy.rights.title": {en: "Your rights", ms: "Hak anda"},
	"privacy.rights.p1": {
		en: "You can ask for a copy of your data, ask us to correct it (your name included), or ask us to delete your account, which also withdraws your consent. Email us from the address on your account, so that we know the request is yours, and we will do it within 21 days.",
		ms: "Anda boleh meminta salinan data anda, meminta kami membetulkannya (termasuk nama anda), atau meminta kami memadam akaun anda, yang juga menarik balik persetujuan anda. E-mel kami daripada alamat pada akaun anda, supaya kami tahu permintaan itu daripada anda, dan kami akan melaksanakannya dalam tempoh 21 hari.",
	},
	"privacy.rights.p2": {
		en: "Deleting your account removes everything on it, your Certificates included, so download your Certificates first.",
		ms: "Memadam akaun anda akan membuang semua yang ada padanya, termasuk Sijil anda, jadi muat turun Sijil anda dahulu.",
	},
	"privacy.changes.title": {en: "Changes to this notice", ms: "Perubahan pada notis ini"},
	"privacy.changes.p": {
		en: "If we ever want to use your data for a new purpose, we will tell you before it applies. Whenever this notice changes, the date at the top changes too.",
		ms: "Jika kami ingin menggunakan data anda untuk tujuan baharu, kami akan memberitahu anda sebelum ia berkuat kuasa. Setiap kali notis ini berubah, tarikh di bahagian atas juga berubah.",
	},
	"privacy.contact.title": {en: "Contact us about your data", ms: "Hubungi kami mengenai data anda"},
	"privacy.contact.p": {
		en: "For a copy, a correction, a deletion or any question about your data, contact The Cave Consult:",
		ms: "Untuk salinan, pembetulan, pemadaman atau sebarang soalan mengenai data anda, hubungi The Cave Consult:",
	},
	// ---- Privacy notice and the Register consent line: end ----

	// Layout: Admin sidebar
	"admin.brand":              {en: "CAVELMS admin", ms: "Pentadbir CAVELMS"},
	"admin.nav":                {en: "Admin", ms: "Pentadbir"},
	"admin.side.dashboard":     {en: "Dashboard", ms: "Dashboard"},
	"admin.side.courses":       {en: "Courses", ms: "Kursus"},
	"admin.side.gallery":       {en: "Gallery", ms: "Galeri"},
	"admin.side.certificates":  {en: "Certificates", ms: "Sijil"},
	"admin.side.cert-details":  {en: "Certificate details", ms: "Butiran Sijil"},
	"admin.side.plans":         {en: "Lab Plans", ms: "Pelan Lab"},
	"admin.side.deployments":   {en: "Deployments", ms: "Deployment"},
	"admin.side.cavetext":      {en: "CaveText Learners", ms: "Pelajar CaveText"},
	"admin.side.cavetext-set":  {en: "CaveText settings", ms: "Tetapan CaveText"},
	"admin.side.cavetext-caps": {en: "CaveText caps", ms: "Had CaveText"},
	"admin.side.log":           {en: "System log", ms: "Log sistem"},
	"admin.side.view-site":     {en: "View learning site", ms: "Lihat laman pembelajaran"},

	// Counts, for the CaveText pages to come
	"cavetext.snippet-count.one":   {en: "%d Snippet", ms: "%d Snippet"},
	"cavetext.snippet-count.other": {en: "%d Snippets", ms: "%d Snippet"},

	// Home page: Coach Adam's write-up in Malay; the English is its translation, for him to proofread.
	"home.title":         {en: "Welcome to CAVELMS", ms: "Selamat Datang ke CAVELMS"},
	"home.lede":          {en: "A Generative AI Digital Learning Platform for Today's and Tomorrow's World of Work.", ms: "Platform Pembelajaran Digital Generative AI untuk Dunia Kerja Masa Kini dan Masa Hadapan."},
	"home.cta.classes":   {en: "Join Online Classes", ms: "Join Online Classes"},
	"home.cta.ebooks":    {en: "Explore eBooks", ms: "Explore eBooks"},
	"home.cta.dashboard": {en: "Learn AI Dashboard Development", ms: "Learn AI Dashboard Development"},
	"home.coach-alt":     {en: "Coach Adam, CEO of The Cave Consult", ms: "Coach Adam, CEO The Cave Consult"},
	"home.intro.1": {
		en: "CAVELMS is a digital learning platform built by The Cave Consult (TCC) to help educators, civil servants, professionals and organisations master Generative Artificial Intelligence (Generative AI) in a way that is practical, systematic and easy to understand.",
		ms: "CAVELMS ialah platform pembelajaran digital yang dibangunkan oleh The Cave Consult (TCC) untuk membantu pendidik, penjawat awam, profesional dan organisasi menguasai teknologi Generative Artificial Intelligence (Generative AI) secara praktikal, sistematik dan mudah difahami.",
	},
	"home.intro.2": {
		en: "We believe that learning AI should not stop at knowing about the technology, or at being able to produce content with prompts alone.",
		ms: "Kami percaya bahawa pembelajaran AI tidak sepatutnya terhenti pada pengetahuan tentang teknologi atau kebolehan menghasilkan kandungan menggunakan arahan prompt semata-mata.",
	},
	"home.intro.3": {
		en: "Instead, AI skills must turn into the ability to solve real problems, raise productivity, analyse information, make decisions and build digital solutions that can be used on an ongoing basis.",
		ms: "Sebaliknya, kemahiran AI perlu diterjemahkan kepada keupayaan menyelesaikan masalah sebenar, meningkatkan produktiviti, menganalisis maklumat, membuat keputusan dan membangunkan penyelesaian digital yang boleh digunakan secara berterusan.",
	},
	"home.intro.4": {
		en: "Through CAVELMS, users can explore a wide range of learning materials, digital eBooks, online classes, tutorials and AI competency programmes designed for self-paced learning and for use at work.",
		ms: "Melalui CAVELMS, pengguna boleh meneroka pelbagai bahan pembelajaran, eBook digital, kelas online, tutorial dan program pembangunan kompetensi AI yang direka untuk pembelajaran kendiri serta aplikasi dalam pekerjaan.",
	},
	"home.pull": {
		en: "Learning AI is not just knowing how to use it. Learning AI means being able to create something truly useful.",
		ms: "Belajar AI bukan sekadar tahu menggunakannya. Belajar AI bermaksud mampu menghasilkan sesuatu yang benar-benar berguna.",
	},
	"home.learn.title":   {en: "What Can You Learn at CAVELMS?", ms: "Apa Yang Boleh Dipelajari di CAVELMS?"},
	"home.learn.1.title": {en: "Generative AI & Workplace Productivity", ms: "Generative AI & Workplace Productivity"},
	"home.learn.1.p1": {
		en: "Learn how to use ChatGPT, Google Gemini and other Generative AI tools to make daily tasks easier, produce documents, prepare reports, analyse information and work more efficiently.",
		ms: "Pelajari cara menggunakan ChatGPT, Google Gemini dan pelbagai teknologi Generative AI untuk memudahkan tugasan harian, menghasilkan dokumen, menyediakan laporan, menganalisis maklumat dan meningkatkan kecekapan kerja.",
	},
	"home.learn.1.p2": {
		en: "The learning is built around real work situations, so users can put what they learn to use straight away.",
		ms: "Pembelajaran disusun berdasarkan situasi pekerjaan sebenar supaya pengguna dapat terus mengaplikasikan kemahiran yang dipelajari.",
	},
	"home.learn.2.title": {en: "Digital eBooks & Practical Learning Resources", ms: "eBook Digital & Practical Learning Resources"},
	"home.learn.2.p1": {
		en: "Explore a collection of digital eBooks that combine explanations of concepts, step-by-step guides, infographics, worked examples and practical exercises.",
		ms: "Terokai koleksi eBook digital yang menggabungkan penerangan konsep, panduan langkah demi langkah, infografik, contoh aplikasi dan latihan praktikal.",
	},
	"home.learn.2.p2": {
		en: "Every learning resource is designed to help users understand a skill at their own level and their own pace.",
		ms: "Setiap bahan pembelajaran direka untuk membantu pengguna memahami sesuatu kemahiran mengikut kemampuan dan masa masing-masing.",
	},
	"home.learn.3.title": {en: "Online Classes & AI Masterclass", ms: "Online Classes & AI Masterclass"},
	"home.learn.3.p1": {
		en: "Build your competence through online classes, structured tutorials, live learning sessions and masterclass programmes.",
		ms: "Tingkatkan kompetensi melalui kelas online, tutorial berstruktur, sesi pembelajaran langsung dan program masterclass.",
	},
	"home.learn.3.p2": {
		en: "The programmes cover the basics of Generative AI, Prompt Engineering, AI Productivity, Data Analytics, Dashboard Development and the use of AI across many fields of work.",
		ms: "Program merangkumi asas Generative AI, Prompt Engineering, AI Productivity, Data Analytics, Dashboard Development dan aplikasi AI dalam pelbagai bidang pekerjaan.",
	},
	"home.learn.4.title": {en: "AI Dashboard Development & Data Analytics", ms: "AI Dashboard Development & Data Analytics"},
	"home.learn.4.p1": {
		en: "Learn to build dashboards with ChatGPT and Gemini Canvas, without needing deep programming experience.",
		ms: "Belajar membina dashboard menggunakan ChatGPT dan Gemini Canvas tanpa perlu mempunyai pengalaman pengaturcaraan yang mendalam.",
	},
	"home.learn.4.p2": {
		en: "Users are introduced to two main approaches to building dashboards.",
		ms: "Pengguna akan diperkenalkan kepada dua pendekatan utama pembangunan dashboard.",
	},
	"home.static.title": {en: "Static Dashboard — Data Visualization & Reporting", ms: "Static Dashboard — Data Visualization & Reporting"},
	"home.static.p1": {
		en: "A Static Dashboard suits reports, visualisations and analysis based on a dataset that has already been prepared.",
		ms: "Static Dashboard sesuai untuk menghasilkan laporan, visualisasi dan analisis berdasarkan set data yang telah disediakan.",
	},
	"home.static.p2": {
		en: "Participants learn how to turn Excel or CSV data into a dashboard with KPIs, charts, tables and analytic summaries.",
		ms: "Peserta mempelajari cara menukar data Excel atau CSV kepada dashboard yang mengandungi KPI, graf, jadual dan ringkasan analitik.",
	},
	"home.static.p3": {
		en: "This approach suits periodic reports, management presentations and analysis that does not need data to be stored or updated continuously.",
		ms: "Pendekatan ini sesuai untuk laporan berkala, pembentangan pengurusan dan analisis yang tidak memerlukan penyimpanan atau kemas kini data secara berterusan.",
	},
	"home.dynamic.title": {en: "Dynamic Dashboard — Live Data & Sustainable Applications", ms: "Dynamic Dashboard — Live Data & Sustainable Applications"},
	"home.dynamic.p1": {
		en: "A Dynamic Dashboard takes the learning to the next level.",
		ms: "Dynamic Dashboard membawa pembelajaran ke tahap seterusnya.",
	},
	"home.dynamic.p2": {
		en: "Participants learn how to build dashboards that take in new data, update records, store information in a database and show the latest analytics.",
		ms: "Peserta mempelajari cara membangunkan dashboard yang boleh menerima data baharu, mengemas kini rekod, menyimpan maklumat dalam database dan memaparkan analitik terkini.",
	},
	"home.dynamic.p3": {
		en: "The learning also introduces GitHub for managing code, Vercel for publishing web applications, and database integration such as Supabase.",
		ms: "Pembelajaran turut memperkenalkan penggunaan GitHub untuk pengurusan kod, Vercel untuk penerbitan aplikasi web serta integrasi database seperti Supabase.",
	},
	"home.dynamic.p4": {
		en: "The aim is to help users build dashboards that not only work when they are made, but can also be used, managed and maintained after the training.",
		ms: "Matlamatnya adalah untuk membantu pengguna membangunkan dashboard yang bukan sahaja berfungsi ketika dihasilkan, tetapi juga boleh digunakan, diurus dan diselenggara selepas latihan.",
	},
	"home.learn.5.title": {en: "AI Skills for Educators & Professionals", ms: "AI Skills for Educators & Professionals"},
	"home.learn.5.p1": {
		en: "CAVELMS also focuses on the use of AI in education, organisational management, administration, human resource development and leadership.",
		ms: "CAVELMS turut memberi tumpuan kepada aplikasi AI dalam pendidikan, pengurusan organisasi, pentadbiran, pembangunan sumber manusia dan kepimpinan.",
	},
	"home.learn.5.p2": {
		en: "Learning grounded in real needs helps users build the confidence to use AI responsibly, effectively and in ways that fit their own roles.",
		ms: "Pembelajaran berasaskan keperluan sebenar membantu pengguna membina keyakinan menggunakan AI secara bertanggungjawab, berkesan dan relevan dengan peranan masing-masing.",
	},
	"home.apply.title": {en: "From Learning to Real Application", ms: "Daripada Pembelajaran kepada Aplikasi Sebenar"},
	"home.apply.p1": {
		en: "At CAVELMS, learning does not end when a tutorial has been watched or an eBook has been read.",
		ms: "Di CAVELMS, pembelajaran tidak berakhir apabila sesuatu tutorial selesai ditonton atau eBook selesai dibaca.",
	},
	"home.apply.p2": {
		en: "We encourage users to try, build, test and use what they have learned to create solutions that bring value to their work and their organisations.",
		ms: "Kami menggalakkan pengguna untuk mencuba, membina, menguji dan menggunakan pengetahuan yang diperoleh bagi menghasilkan penyelesaian yang memberi nilai kepada pekerjaan dan organisasi.",
	},
	"home.apply.note":       {en: "This is the CAVELMS approach to learning.", ms: "Inilah pendekatan pembelajaran CAVELMS."},
	"home.programmes.title": {en: "Past programmes", ms: "Program terdahulu"},
	"home.programmes.lede": {
		en: "Programmes The Cave Consult has run for organisations and the public.",
		ms: "Program yang pernah dijalankan oleh The Cave Consult untuk organisasi dan orang awam.",
	},
	"home.gallery.title": {en: "From the gallery", ms: "Dari Galeri"},
	"home.gallery.lede":  {en: "Videos, ebooks and pictures, open to everyone.", ms: "Video, eBook dan gambar, terbuka kepada semua."},
	"home.gallery.open":  {en: "Open the gallery", ms: "Buka Galeri"},
	"home.start.title":   {en: "Start Your AI Learning Journey", ms: "Mulakan Perjalanan Pembelajaran AI"},
	"home.start.p": {
		en: "Whether you are new to Generative AI or want to build your skills in developing digital applications, CAVELMS gives you a flexible learning space focused on practical application.",
		ms: "Sama ada baru mengenali Generative AI atau ingin meningkatkan kemahiran membangunkan aplikasi digital, CAVELMS menyediakan ruang pembelajaran yang fleksibel dan berorientasikan aplikasi praktikal.",
	},
}
