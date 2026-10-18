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
  ],

  /* Admin Brief per Course (saved in demo state once edited). */
  briefs: {
    genai: 'Topic: using generative AI at work for business staff, from writing emails and reports to customer replies, marketing and checking results. Audience: busy staff in small businesses with no technical background. Tone: practical, friendly and short, no jargon. Each Lesson ends with one thing to try at work.'
  },

  /* Scripted AI writing. Canned output only: nothing is sent anywhere. */
  aiOutline: [
    { title: 'Your first AI assistant at work', objective: 'Run a simple task with an AI tool and judge the answer.' },
    { title: 'Writing prompts that work', objective: 'Write a prompt with a role, a task, context and a format.' },
    { title: 'Turning rough notes into emails', objective: 'Turn a rough idea into a clear email you can send.' },
    { title: 'Writing reports faster', objective: 'Draft a short report from your notes in under 30 minutes.' },
    { title: 'Summarising long documents', objective: 'Summarise a long document into five key points.' },
    { title: 'Meeting notes and action lists', objective: 'Turn meeting notes into an action list with owners.' },
    { title: 'Spreadsheets with AI help', objective: 'Ask AI to explain a formula and write one you can use.' },
    { title: 'Product descriptions for your shop', objective: 'Write product descriptions in your own voice.' },
    { title: 'Planning a week of social posts', objective: 'Plan and write a week of posts with your brand in mind.' },
    { title: 'Drafting customer replies', objective: 'Draft a customer reply that is polite and accurate.' },
    { title: 'Checking AI answers before use', objective: 'Check the facts in an AI answer before you use it.' },
    { title: 'Protecting customer data', objective: 'Name the data you must never paste into a public AI tool.' },
    { title: 'Simple visuals for marketing', objective: 'Make a simple marketing visual with an AI image tool.' },
    { title: 'Short videos from a script', objective: 'Turn a short script into a video with an AI tool.' },
    { title: 'Research with AI', objective: 'Use AI to list sources, then check each one yourself.' },
    { title: 'Translating for customers', objective: 'Translate a message and have it checked before it goes out.' },
    { title: 'Drafting quotes and invoices', objective: 'Draft a quote template and check every number in it.' },
    { title: 'A weekly routine with AI', objective: 'Build a weekly routine that uses AI for admin tasks.' },
    { title: 'Sharing prompts with your team', objective: 'Build a small prompt library your team can reuse.' },
    { title: 'Simple rules for AI at work', objective: 'Write three rules your team follows when using AI.' },
    { title: 'Measuring time saved', objective: 'Track the time AI saves on one task over a month.' },
    { title: 'Spotting biased wording', objective: 'Spot unfair or biased wording in AI output.' },
    { title: 'Copyright basics', objective: 'Know when AI output needs a source or permission.' },
    { title: 'Automating a repeat task', objective: 'Set up a simple AI workflow for one task you repeat.' },
    { title: 'What a customer chatbot can do', objective: 'Explain what a customer chatbot can and cannot do for you.' },
    { title: 'Review before anything goes out', objective: 'Set a review step before anything reaches a customer.' },
    { title: 'Common AI mistakes', objective: 'Recognise the five most common mistakes AI tools make.' },
    { title: 'Choosing the right AI tool', objective: 'Compare two AI tools by task and by how they handle data.' },
    { title: 'Your 30-day AI action plan', objective: 'Write a 30-day plan that puts AI into your business.' },
    { title: 'Keeping your skills current', objective: 'Find ways to keep learning as AI tools change.' }
  ],

  /* Lesson text in Markdown. {title} is replaced with the Lesson title. */
  aiLessonText: [
    '## {title}\n\nThis Lesson takes about five minutes. Read it once, then try the example with a real task from your week.\n\n- Pick one task you repeat every week.\n- Use the four-part prompt from the earlier Lesson.\n- Check the result before you use it.\n\n**Try it now.** Write one prompt for this task, then ask for one change and compare the two answers.',
    '## {title}\n\nAI tools give better answers when you give them clear instructions. In this Lesson you practise that habit.\n\n- Say who the AI should be.\n- Say exactly what you need.\n- Add the facts it needs to know.\n- Describe the format you want.\n\n**Check your work.** Read the answer as your customer would before you send it.',
    '## {title}\n\nThis is a short reading for busy people. Keep it to one task today and note what changed.\n\n- Write the task down in one sentence.\n- Ask the AI for a first draft.\n- Ask for one or two changes, not a fresh start.\n- Read the final version out loud before you use it.'
  ],

  /* Scripted exam questions. Correct is the index of the right option. */
  aiQuestions: [
    { q: 'What should you do before sending an AI-written customer email?', options: ['Send it straight away to save time', 'Read it and check the facts and tone', 'Add more adjectives', 'Ask the AI to sign it'], correct: 1 },
    { q: 'Which is the best use of AI for a busy manager?', options: ['Making final pay decisions alone', 'Drafting a first version of a team update', 'Replacing every staff meeting', 'Choosing legal wording without review'], correct: 1 },
    { q: 'Why ask the AI for changes instead of starting again?', options: ['It forgets the last answer', 'It keeps the useful parts of the first answer', 'Starting again is not allowed', 'Each new chat must be shorter'], correct: 1 },
    { q: 'Why do AI tools sometimes give wrong facts?', options: ['They always check a database first', 'They predict likely words and can sound sure while wrong', 'They only use the last email you sent', 'They are not allowed to answer numbers'], correct: 1 },
    { q: 'Which detail belongs in a good prompt?', options: ['Only the word “help”', 'Who the answer is for and what format you want', 'A guess at the answer', 'Your password'], correct: 1 },
    { q: 'Before you paste a document into a public AI tool, what should you check?', options: ['Its font size', 'Whether it contains customers’ personal data', 'Its page count', 'Its file colour'], correct: 1 },
    { q: 'What should a good meeting-notes prompt ask for?', options: ['A poem about the meeting', 'Decisions, action items and owners', 'The weather on the day', 'Each attendee’s hobbies'], correct: 1 },
    { q: 'What is the best first step when you are unsure about an AI answer?', options: ['Accept it because it sounds sure', 'Check it against a trusted source', 'Delete the question', 'Ask it to repeat itself'], correct: 1 },
    { q: 'Which tone instruction is clearest?', options: ['Make it nice', 'Friendly and short, under 100 words', 'Do your best', 'Whatever you think is good'], correct: 1 },
    { q: 'What makes an AI-written social post safer to publish?', options: ['Checking the claims and brand voice first', 'Posting it at midnight', 'Using the longest version', 'Not reading it'], correct: 0 },
    { q: 'Which is a sign that an AI answer needs checking?', options: ['It has a specific statistic with no source', 'It is short', 'It uses the word “please”', 'It was written in one go'], correct: 0 },
    { q: 'What is a good way to build a team AI routine?', options: ['Let everyone guess the rules', 'Share tested prompts and agree on review rules', 'Forbid every AI tool', 'Use one prompt for every task forever'], correct: 1 }
  ],

  /* Messages when the AI cannot help. None of them is the Admin's mistake. */
  aiErrors: {
    unavailable: { title: 'AI writing is unavailable right now.', text: 'The AI service did not respond. This is not a mistake in your Course and nothing has changed. Try again in a few minutes.' },
    notConfigured: { title: 'AI writing is not set up on this site yet.', text: 'The site still needs its AI connection switched on. This is a setup step for the site, not something you did. Nothing has changed.' },
    overLimit: { title: 'This site has reached its AI writing spending limit for the month.', text: 'No more AI writing runs until the limit resets. This is not a mistake in your Course. Nothing has changed.' }
  }
};
