/* PROTOTYPE sample data: one source for every screen. Throwaway.
   Course titles, photos, testimonials and contact details are The Cave Consult's own
   public material; people, numbers and certificate IDs are invented samples. */

window.DATA = {
  org: {
    name: 'The Cave Consult',
    portal: 'Learning',
    arm: 'ISDeT (Industry Skill Development & Training Centre)',
    partner: 'UTM CAIRO, Universiti Teknologi Malaysia',
    phone: '011-3566 7300',
    email: 'admin@thecaveconsult.com',
    address: 'Lot 11B Bazar Enggang Keramat, Lorong Enggang 4, Taman Keramat, 54200 Kuala Lumpur',
    checkBase: 'learn.thecaveconsult.com/check/',
    signatory: 'Mohd Azlan Rahim',
    signatoryTitle: 'Director of Training, ISDeT'
  },

  courses: [
    {
      id: 'genai', title: 'Leveraging Generative AI for Business', open: true, start: '3 Nov 2026',
      img: 'img/class-whiteboard.jpg', lessons: 6, exam: { questions: 5, pass: 70, minutes: 20, attempts: 3 },
      enrolled: 186, completed: 64,
      blurb: 'Use ChatGPT and other AI tools to save hours on everyday business work, from emails and reports to marketing content.',
      learn: ['Write prompts that get a useful first draft', 'Turn AI output into emails, reports and posts you can send', 'Check facts and keep customer data safe', 'Build a repeatable AI routine for your team']
    },
    {
      id: 'productivity', title: 'Boosting Workplace Productivity With Gen-AI', open: true, start: '17 Nov 2026',
      img: 'img/class-mara-lab.jpg', lessons: 5, exam: { questions: 10, pass: 70, minutes: 25, attempts: null },
      enrolled: 142, completed: 51,
      blurb: 'Cut the time you spend on meetings notes, spreadsheets and admin with simple AI habits you can use from day one.'
    },
    {
      id: 'content', title: 'Content Marketing With AI Secret Weapon', open: true, start: '8 Dec 2026',
      img: 'img/class-content.jpg', lessons: 7, exam: { questions: 10, pass: 70, minutes: 30, attempts: 3 },
      enrolled: 121, completed: 38,
      blurb: 'Plan a month of posts in an afternoon. Ideas, captions, visuals and short videos made with AI, in your brand voice.'
    },
    {
      id: 'website', title: 'Website in Minutes With AI + Ecommerce', open: true, start: '12 Jan 2027',
      img: 'img/hero-learner.jpg', lessons: 5, exam: null,
      enrolled: 97, completed: 22,
      blurb: 'Build a simple business website and online shop with AI builders, then take your first order. No coding needed.'
    },
    {
      id: 'cpgai', title: 'Certified Professional in Generative AI (CPGAI)', open: true, start: '2 Feb 2027',
      img: 'img/class-speaker.jpg', lessons: 10, exam: { questions: 30, pass: 80, minutes: 45, attempts: 2 },
      enrolled: 64, completed: 9,
      blurb: 'Our most complete programme, from AI foundations to strategy and governance, ending with a certification exam.'
    },
    {
      id: 'pesara', title: 'Generatif AI Untuk Bakal Pesara ATM', open: false, start: '5 May 2026',
      img: 'img/class-auditorium.jpg', lessons: 6, exam: { questions: 10, pass: 60, minutes: 20, attempts: null },
      enrolled: 38, completed: 31,
      blurb: 'Kursus AI generatif untuk bakal pesara ATM: guna AI untuk memulakan perniagaan kecil dan kerjaya baharu.'
    }
  ],

  /* The demo course, lesson by lesson. */
  lessons: {
    genai: [
      { n: 1, title: 'What generative AI can do', format: 'video', length: '8 min', img: 'img/class-whiteboard.jpg' },
      { n: 2, title: 'Writing good prompts', format: 'text', length: '6 min read' },
      { n: 3, title: 'AI for emails and reports', format: 'video', length: '12 min', img: 'img/class-content.jpg' },
      { n: 4, title: 'AI for marketing content', format: 'video', length: '10 min', img: 'img/class-workshop.jpg' },
      { n: 5, title: 'Risks and responsible use', format: 'ebook', length: 'PDF, 14 pages' },
      { n: 6, title: 'Building your AI workflow', format: 'text', length: '5 min read' }
    ]
  },

  /* Lesson titles for the other courses (outline only). */
  outlines: {
    productivity: ['Where your working hours go', 'AI for meeting notes and minutes', 'Spreadsheets with AI help', 'Drafting documents faster', 'Your weekly AI routine'],
    content: ['Finding content ideas with AI', 'Writing captions in your brand voice', 'Designing visuals with AI', 'Short videos made simple', 'Planning a content calendar', 'Measuring what works', 'Your 30-day content plan'],
    website: ['Choosing an AI website builder', 'Your homepage in 15 minutes', 'Adding products and prices', 'Taking payments online', 'Launching and getting found'],
    cpgai: ['AI foundations', 'How large language models work', 'Prompt engineering in depth', 'AI for analysis and decisions', 'Automating workflows', 'AI in marketing and sales', 'Data privacy and the law', 'Ethics and responsible AI', 'Building an AI strategy', 'Leading AI change in your organisation'],
    pesara: ['Apa itu AI generatif', 'Menulis arahan (prompt) yang baik', 'AI untuk idea perniagaan', 'Pemasaran dengan AI', 'Keselamatan dan penipuan dalam talian', 'Pelan tindakan anda']
  },

  exam: {
    genai: [
      { q: 'Which prompt is most likely to get a useful first draft?', options: [
        'Write an email.',
        'Write a friendly 120-word email to a client confirming Tuesday’s 10am meeting, and ask them to bring last quarter’s sales figures.',
        'Email please, make it good.',
        'Help me.'], correct: 1 },
      { q: 'An AI tool gives you a statistic for your report. What should you do before using it?', options: [
        'Use it as it is. AI tools are always right.',
        'Round it so it looks neater.',
        'Check it against a trusted source.',
        'Leave out where it came from.'], correct: 2 },
      { q: 'Which of these should you NOT paste into a public AI chatbot?', options: [
        'Your customer list with phone numbers',
        'A press release you already published',
        'A general job description',
        'A blog post from your website'], correct: 0 },
      { q: 'You don’t like the AI’s answer. What usually works best?', options: [
        'Start a completely new topic',
        'Ask the exact same question again',
        'Accept it anyway',
        'Tell it what to change and ask it to revise'], correct: 3 },
      { q: '“Generative AI” is AI that…', options: [
        'only sorts data into categories',
        'creates new content such as text, images or audio',
        'replaces the need for the internet',
        'only works without an internet connection'], correct: 1 }
    ]
  },

  /* The demo Learner. */
  me: { name: 'Nurul Aina binti Rahman', first: 'Nurul', initials: 'NA', email: 'nurulaina.rahman@gmail.com' },

  /* Starting state for the demo Learner (the tour resets to this). */
  start: {
    joined: ['genai', 'content', 'website'],
    done: { genai: [1, 2, 3, 4], content: [1, 2, 3, 4, 5, 6, 7], website: [] },
    certs: { content: 'TCC-4M8P-Q2RW' }
  },

  learners: [
    { name: 'Nurul Aina binti Rahman', email: 'nurulaina.rahman@gmail.com', joined: '2 Sep 2026', courses: 3, progress: 4, best: null, cert: null },
    { name: 'Tan Mei Ling', email: 'meiling.tan@outlook.com', joined: '28 Aug 2026', courses: 2, progress: 6, best: 90, cert: 'TCC-8F2D-L6PX' },
    { name: 'Arjun Nair', email: 'arjun.nair@techwave.my', joined: '30 Aug 2026', courses: 1, progress: 6, best: 60, cert: null },
    { name: 'Muhammad Hafizuddin Hassan', email: 'hafizuddin.h@gmail.com', joined: '1 Sep 2026', courses: 2, progress: 5, best: null, cert: null },
    { name: 'Siti Khadijah Omar', email: 'khadijah.omar@mara.gov.my', joined: '3 Sep 2026', courses: 4, progress: 6, best: 100, cert: 'TCC-2R7N-B5QH' },
    { name: 'Lim Wei Jie', email: 'weijie.lim@gmail.com', joined: '4 Sep 2026', courses: 1, progress: 6, best: 80, cert: 'TCC-3H6T-K8VC' },
    { name: 'Kavitha Subramaniam', email: 'kavitha.s@yahoo.com', joined: '7 Sep 2026', courses: 2, progress: 2, best: null, cert: null },
    { name: 'Ahmad Faris Iskandar', email: 'faris.iskandar@gmail.com', joined: '9 Sep 2026', courses: 1, progress: 6, best: 70, cert: 'TCC-6J4W-T9ME' },
    { name: 'Chong Pei Shan', email: 'peishan.chong@sme-hub.my', joined: '12 Sep 2026', courses: 3, progress: 1, best: null, cert: null },
    { name: 'Rajesh Kumar a/l Muthu', email: 'rajesh.muthu@gmail.com', joined: '15 Sep 2026', courses: 1, progress: 0, best: null, cert: null },
    { name: 'Farah Nadia Ismail', email: 'farahnadia@pdrm.gov.my', joined: '18 Sep 2026', courses: 2, progress: 6, best: 50, cert: null },
    { name: 'Daniel Wong Kah Hoe', email: 'daniel.wong@kahhoe.com', joined: '22 Sep 2026', courses: 1, progress: 3, best: null, cert: null }
  ],

  certificates: [
    { id: 'TCC-4M8P-Q2RW', name: 'Nurul Aina binti Rahman', course: 'content', date: '19 Sep 2026', status: 'valid' },
    { id: 'TCC-8F2D-L6PX', name: 'Tan Mei Ling', course: 'genai', date: '14 Oct 2026', status: 'valid' },
    { id: 'TCC-2R7N-B5QH', name: 'Siti Khadijah Omar', course: 'genai', date: '11 Oct 2026', status: 'valid' },
    { id: 'TCC-6J4W-T9ME', name: 'Ahmad Faris Iskandar', course: 'genai', date: '9 Oct 2026', status: 'valid' },
    { id: 'TCC-3H6T-K8VC', name: 'Lim Wei Jie', course: 'genai', date: '7 Oct 2026', status: 'valid', note: 'Replaces TCC-9D2L-X7NB' },
    { id: 'TCC-9D2L-X7NB', name: 'Lim Wee Jie', course: 'genai', date: '6 Oct 2026', status: 'revoked', note: 'Name misspelled. Replaced by TCC-3H6T-K8VC' },
    { id: 'TCC-5C1V-H3KD', name: 'Zulkifli Mohd Noor', course: 'pesara', date: '28 Jun 2026', status: 'valid' },
    { id: 'TCC-7A9Q-R2FS', name: 'Norhayati Abdullah', course: 'pesara', date: '27 Jun 2026', status: 'valid' }
  ],

  gallery: [
    { id: 'g1', title: 'A day in our Generative AI class', format: 'video', meta: '3 min', img: 'img/class-whiteboard.jpg', category: 'Events', visibility: 'everyone', promo: true },
    { id: 'g2', title: 'ISDeT programme guide 2027', format: 'ebook', meta: 'PDF, 24 pages', img: 'img/prospectus.jpg', category: 'Guides', visibility: 'everyone', promo: true },
    { id: 'g3', title: 'Training day at MARA', format: 'picture', meta: '18 pictures', img: 'img/class-mara-lab.jpg', category: 'Events', visibility: 'everyone', promo: true },
    { id: 'g4', title: 'Certificate handover', format: 'picture', meta: '6 pictures', img: 'img/event-handover.jpg', category: 'Events', visibility: 'everyone', promo: false },
    { id: 'g5', title: 'AI for TVET instructors', format: 'picture', meta: '12 pictures', img: 'img/event-group.jpg', category: 'Events', visibility: 'everyone', promo: false },
    { id: 'g6', title: 'Lecture hall session', format: 'video', meta: '2 min', img: 'img/class-lecture-hall.jpg', category: 'Events', visibility: 'everyone', promo: false },
    { id: 'g7', title: 'Prompt cheat sheet', format: 'ebook', meta: 'PDF, 4 pages', img: 'img/class-content.jpg', category: 'Guides', visibility: 'learners', promo: false },
    { id: 'g8', title: 'What generative AI can do', format: 'video', meta: '8 min', img: 'img/class-workshop.jpg', category: 'Course material', visibility: 'members', promo: false },
    { id: 'g9', title: 'Risks and responsible use', format: 'ebook', meta: 'PDF, 14 pages', img: 'img/class-group-screen.jpg', category: 'Course material', visibility: 'members', promo: false }
  ],

  testimonials: [
    { quote: 'Seriously, this class was extraordinary! I learned so many powerful techniques using AI and ChatGPT that I never even imagined before.', name: 'Datin Lyn', place: 'Kuala Lumpur' },
    { quote: 'I gained so many valuable ideas during the class. It truly opened up new perspectives, and now I’m beginning to explore and put into practice the techniques I’ve learned.', name: 'Mr T Mogan Raj', place: 'Alumni' }
  ],

  activity: [
    { who: 'Tan Mei Ling', what: 'passed the exam for Leveraging Generative AI for Business with 90%', when: '12 min ago', icon: 'seal-check' },
    { who: 'Chong Pei Shan', what: 'joined Content Marketing With AI Secret Weapon', when: '40 min ago', icon: 'users' },
    { who: 'Arjun Nair', what: 'scored 60% on Leveraging Generative AI for Business (pass mark 70%)', when: '1 hr ago', icon: 'exam' },
    { who: 'Siti Khadijah Omar', what: 'received certificate TCC-2R7N-B5QH', when: '3 hr ago', icon: 'certificate' },
    { who: 'Daniel Wong Kah Hoe', what: 'registered', when: 'Yesterday', icon: 'user-circle' }
  ]
};
