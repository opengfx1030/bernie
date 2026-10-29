package glossary

import "testing"

func TestEveryWordHasAMeaning(t *testing.T) {
	if len(Words) == 0 {
		t.Fatal("the glossary is empty")
	}
	seen := map[string]bool{}
	for _, w := range Words {
		if w.Term == "" || w.Meaning == "" {
			t.Errorf("the word %+v has no term or no meaning", w)
		}
		if seen[w.Term] {
			t.Errorf("the word %q is twice in the glossary", w.Term)
		}
		seen[w.Term] = true
	}
}

func TestMeaningOfAKnownWord(t *testing.T) {
	if Meaning("HIP") == "" {
		t.Error("HIP has no meaning")
	}
	if Meaning("a word nobody uses") != "" {
		t.Error("an unknown word has a meaning")
	}
}

func TestTermsAreTheWords(t *testing.T) {
	terms := Terms()
	if len(terms) != len(Words) {
		t.Errorf("the terms are %d, want %d", len(terms), len(Words))
	}
}
