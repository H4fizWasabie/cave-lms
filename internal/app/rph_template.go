package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/text"
)

// The RPH prompt (ADR 0029) is built from this fixed template, in code, with no AI call: the Teacher's lesson details and
// the standards they ticked are filled in, and the words around them are the ones below.
//
// DRAFT for the client's approval: every fixed instruction in rphWords, in Malay and in English, is the draft wording of
// the RPH prompt, which the client reads and approves before the Teachers Portal launches (spec #568, Further Notes).
// This file is the one place to review them; change the wording here and nowhere else.
var rphWords = map[string]struct {
	Intro                                              string
	Lesson, Subject, Year, Date, Time, Class, Students string // the lesson's details, each a label
	Standards, Group, Topic, SK, SP, Notes             string // the DSKP's standards and the Teacher's notes
	SectionsIntro                                      string
	Sections                                           []string // what the RPH has, in order
	Rules                                              string
	Logo                                               string // asked for only when the Teacher ticks their School logo
}{
	"ms": {
		Intro:         "Anda ialah seorang guru berpengalaman dan pakar kurikulum Malaysia. Tolong tulis Rancangan Pelajaran Harian (RPH) yang lengkap untuk satu pengajaran dan pembelajaran, berdasarkan standard daripada Dokumen Standard Kurikulum dan Pentaksiran (DSKP) di bawah.",
		Lesson:        "Maklumat pelajaran",
		Subject:       "Mata pelajaran",
		Year:          "Tahun",
		Date:          "Tarikh",
		Time:          "Masa atau tempoh",
		Class:         "Kelas",
		Students:      "Bilangan murid",
		Standards:     "Standard daripada DSKP",
		Group:         "Tema atau bidang",
		Topic:         "Tajuk",
		SK:            "Standard Kandungan (SK)",
		SP:            "Standard Pembelajaran (SP)",
		Notes:         "Catatan guru",
		SectionsIntro: "Tulis RPH dengan bahagian berikut, mengikut urutan ini:",
		Sections: []string{
			"Objektif: objektif pembelajaran yang selari dengan standard di atas, bermula dengan \"Pada akhir pelajaran, murid dapat ...\".",
			"Kriteria kejayaan: apa yang murid boleh buat atau tunjukkan jika objektif tercapai.",
			"Aktiviti: langkah demi langkah dengan anggaran masa, dalam tiga bahagian: set induksi, perkembangan dan penutup.",
			"Bahan bantu mengajar (BBM): bahan yang diperlukan.",
			"Penilaian: cara guru menilai sama ada murid mencapai objektif, selari dengan standard di atas.",
			"Refleksi: soalan panduan untuk guru menulis refleksi selepas pelajaran.",
		},
		Rules: "Ikut hanya standard di atas dan jangan menambah standard lain. Sesuaikan aktiviti dengan masa dan bilangan murid, dan ambil kira catatan guru. Tulis RPH dalam Bahasa Melayu.",
		Logo:  "Letakkan logo sekolah yang dilampirkan di bahagian atas RPH. Pulangkan RPH sebagai dokumen yang boleh dimuat turun (Word atau PDF) supaya logo itu dapat dipaparkan.",
	},
	"en": {
		Intro:         "You are an experienced teacher and an expert in the Malaysian curriculum. Please write a complete Rancangan Pelajaran Harian (RPH, a daily lesson plan) for one lesson, based on the standards from the Dokumen Standard Kurikulum dan Pentaksiran (DSKP) below.",
		Lesson:        "Lesson details",
		Subject:       "Subject",
		Year:          "Year",
		Date:          "Date",
		Time:          "Time or duration",
		Class:         "Class",
		Students:      "Number of pupils (murid)",
		Standards:     "Standards from the DSKP",
		Group:         "Theme or area (tema or bidang)",
		Topic:         "Topic (tajuk)",
		SK:            "Content Standard (Standard Kandungan, SK)",
		SP:            "Learning Standard (Standard Pembelajaran, SP)",
		Notes:         "Teacher's notes",
		SectionsIntro: "Write the RPH with these sections, in this order:",
		Sections: []string{
			"Objective (objektif): learning objectives that match the standards above, starting with \"By the end of the lesson, pupils will be able to ...\".",
			"Success criteria (kriteria kejayaan): what pupils can do or show if the objectives are met.",
			"Activities (aktiviti): step by step with estimated times, in three parts: set induction (set induksi), development (perkembangan) and closure (penutup).",
			"Teaching aids (bahan bantu mengajar, BBM): the materials needed.",
			"Assessment (penilaian): how the teacher checks whether pupils met the objectives, matching the standards above.",
			"Reflection (refleksi): guiding questions for the teacher to write a reflection after the lesson.",
		},
		Rules: "Follow only the standards above and do not add other standards. Fit the activities to the time and the number of pupils, and take the teacher's notes into account. Write the RPH in English.",
		Logo:  "Place the attached school logo at the top of the RPH. Return the RPH as a downloadable document (Word or PDF) so that the logo can appear.",
	},
}

// rphLesson is what the Teacher typed and ticked.
type rphLesson struct {
	Subject, Year        string // the DSKP's subject and year code
	Date                 time.Time
	Time, Class          string
	Students             int
	Notes                string
	Group, Topic, SKCode string // the SK's tema or bidang, tajuk and code
	SKText               string
	SPs                  []rphSP // the ticked SPs, in the DSKP's order
	Logo                 bool    // the Teacher attaches their School logo in their chatbot
}

// rphPromptText is the RPH prompt for the lesson in lang, "ms" or "en". The standards are quoted as the DSKP has them,
// whatever lang is.
func rphPromptText(lang string, l rphLesson) string {
	w := rphWords[lang]
	var b strings.Builder
	line := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintf(&b, "- %s: %s\n", label, value)
		}
	}
	b.WriteString(w.Intro + "\n\n" + w.Lesson + "\n")
	line(w.Subject, l.Subject)
	line(w.Year, text.T(lang, "teachers.year."+l.Year))
	line(w.Date, text.Date(lang, l.Date))
	line(w.Time, l.Time)
	line(w.Class, l.Class)
	line(w.Students, fmt.Sprint(l.Students))
	b.WriteString("\n" + w.Standards + "\n")
	line(w.Group, l.Group)
	line(w.Topic, l.Topic)
	line(w.SK+" "+l.SKCode, l.SKText)
	for _, sp := range l.SPs {
		line(w.SP+" "+sp.Code, sp.Text)
	}
	if strings.TrimSpace(l.Notes) != "" {
		b.WriteString("\n" + w.Notes + "\n" + strings.TrimSpace(l.Notes) + "\n")
	}
	b.WriteString("\n" + w.SectionsIntro + "\n")
	for i, s := range w.Sections {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	b.WriteString("\n" + w.Rules + "\n")
	if l.Logo {
		b.WriteString("\n" + w.Logo + "\n")
	}
	return b.String()
}
