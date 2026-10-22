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
