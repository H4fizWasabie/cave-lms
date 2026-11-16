package app

import (
	"fmt"
	"strings"
)

// The OPR prompt (spec #578) is built from this fixed core, in code, with no AI call: the Teacher's details, the OPR style the
// Admin wrote and the colour are filled in, and the words around them are the ones below. They are instructions to the
// Teacher's chatbot, so they are in English whatever the page's language is; oprWords only holds the words of the OPR itself,
// which follow the OPR's language (Malay or English). Modelled on the CAVEZON OPR Image Prompt Builder.
//
// DRAFT for the client's approval: every fixed instruction below, and the words in oprWords, are the draft wording of the OPR
// prompt, which the client reads and approves before the Teachers Portal launches (spec #578, Further Notes). This file is
// the one place to review them; change the wording here and nowhere else.

// oprWords are the words the chatbot is told to put in the OPR itself, by the OPR's language, "ms" or "en".
var oprWords = map[string]struct {
	Language     string // the language the OPR's text is in
	DetailsTitle string // the heading of the details table, Section 1
	PreparedBy   string // the footer's labels
	Source       string
}{
	"ms": {Language: "Bahasa Malaysia", DetailsTitle: "MAKLUMAT PROGRAM / AKTIVITI", PreparedBy: "Disediakan oleh", Source: "Sumber"},
	"en": {Language: "English", DetailsTitle: "PROGRAMME / ACTIVITY INFORMATION", PreparedBy: "Prepared by", Source: "Source"},
}

// oprDetails is what the Teacher typed for the OPR (Tajuk is the title; the page has a Title of its own); Time, Target, Extra and Colour are optional.
type oprDetails struct {
	Tajuk, Institution, Date, Time, Place, Target, Reporter, Reference, Extract, Extra, Colour string
	UseLogo                                                                                    bool // the Teacher attaches their School logo in their chatbot
}

// oprPromptText is the OPR prompt for the details in the OPR style, with the OPR's own text in lang, "ms" or "en".
func oprPromptText(lang string, d oprDetails, style oprStyle) string {
	w := oprWords[lang]
	var b strings.Builder
	line := func(label, value string) {
		if value != "" {
			fmt.Fprintf(&b, "- %s: %s\n", label, value)
		}
	}
	fmt.Fprintf(&b, "Create ONE finished, high-resolution A4 portrait infographic image in %s. This is an official ONE-PAGE REPORT (OPR), not a text draft and not a blank template.\n\n", w.Language)
	b.WriteString("SOURCE RULE:\n" +
		"Read the attached reference (a PDF, a text file or a picture) carefully and use ONLY it and the locked details below. Use the locked details exactly as written. " +
		"Do not invent, infer, embellish or insert placeholders. If a requested fact is absent from the reference, omit that point or use a neutral heading without a claim.\n\n")
	b.WriteString("LOCKED DETAILS (use exactly as written):\n")
	line("Main title", "ONE-PAGE REPORT (OPR)")
	line("Programme or report title", d.Tajuk)
	line("Institution", d.Institution)
	line("Date", d.Date)
	line("Time", d.Time)
	line("Venue", d.Place)
	line("Target group", d.Target)
	line("Teacher or reporting officer", d.Reporter)
	line("Reference", d.Reference)
	b.WriteString("\nEXTRACT FROM THE ATTACHED REFERENCE:\n" + d.Extract + "\n\n")
	b.WriteString("SELECTED OPR STYLE: " + style.Name + "\n" + style.Text + "\n\n")
	b.WriteString("CONTENT REQUIREMENTS:\n" +
		"1. Header with the institution name and the exact programme title.\n" +
		"2. Section 1, \"" + w.DetailsTitle + "\": a tidy two-column table containing ALL the locked details above.\n" +
		"3. Extract and organise only content supported by the attached reference, according to the selected OPR style.\n" +
		"4. Use short headings and concise explanations. Do not present national statistics or general policy claims as the achievement of this programme.\n" +
		"5. Footer: \"" + w.PreparedBy + ": " + d.Reporter + "\" and \"" + w.Source + ": " + d.Reference + "\".\n\n")
	if d.UseLogo {
		b.WriteString("SCHOOL LOGO:\nPlace the attached school logo in the header, beside the institution name. Do not redraw or alter it.\n\n")
	}
	b.WriteString("VISUAL DIRECTION:\n")
	if d.Colour != "" {
		b.WriteString("Main colour: " + d.Colour + ". ")
	}
	fmt.Fprintf(&b, "Use excellent information hierarchy, sharp legible %s text, clear section headers, restrained icons, clean tables and generous white space. "+
		"The layout must look ready for official circulation. Ensure every heading and all locked details are spelled exactly as supplied.\n", w.Language)
	if d.Extra != "" {
		b.WriteString("\nADDITIONAL INSTRUCTION:\n" + d.Extra + "\n")
	}
	b.WriteString("\nMANDATORY TWO-STAGE ANALYSIS:\n" +
		"Stage 1: Analyse the attached reference deeply and internally before designing. Identify the exact facts, headings, objectives, activities, dates, names, data, outcomes, strengths, weaknesses and follow-up actions requested above. Resolve conflicts only from the reference and the locked details.\n" +
		"Stage 2: Convert the verified analysis into the selected OPR style. Do not show your working or an analysis draft. Return only the finished OPR infographic image.\n")
	return b.String()
}
