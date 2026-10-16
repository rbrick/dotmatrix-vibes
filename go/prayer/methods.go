package prayer

var (
	//export
	Methods = map[string]Method{
		"MWL":     {Name: "Muslim World League", FajrDegrees: 18, IshaDegrees: 17},
		"ISNA":    {Name: "Islamic Society of North America", FajrDegrees: 15, IshaDegrees: 15},
		"EGAS":    {Name: "Egyptian General Authority of Survey", FajrDegrees: 19.5, IshaDegrees: 17.5},
		"Karachi": {Name: "University of Islamic Sciences, Karachi", FajrDegrees: 18, IshaDegrees: 18},
		"UAE":     {Name: "Dubai (UAE)", FajrDegrees: 18.2, IshaDegrees: 18.2},
		"Kuwait":  {Name: "Kuwait", FajrDegrees: 18, IshaDegrees: 17.5},
	}
)

type Method struct {
	Name        string
	FajrDegrees float64
	IshaDegrees float64
}
