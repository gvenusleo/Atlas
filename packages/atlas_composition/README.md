# atlas_composition

Shared process composition for Atlas applications. This package constructs the
configured providers, tools, storage, prompt builder, and single runtime. It
does not own presentation, protocol handling, or application lifecycle.

Applications may depend on this package from their composition root. Features
and presentation code must receive the resulting runtime through injection.
