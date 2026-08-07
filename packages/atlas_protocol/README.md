# atlas_protocol

The versioned wire protocol used by remote Atlas clients.

Protocol DTOs and codecs are separate from runtime and persistence models. The
package does not own a network transport or local presentation API.
