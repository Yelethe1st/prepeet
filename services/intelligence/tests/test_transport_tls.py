"""The intelligence plane's transport security.

The control plane sends briefs over this hop and receives transcripts back, so
it carries candidate speech. It was plaintext on both ends: the Go client asked
for insecure credentials unconditionally and the server bound only
``add_insecure_port``. The comment beside the Go dial claimed the deployed path
got TLS, which no code anywhere provided.

These tests complete real handshakes against a throwaway certificate authority
rather than inspecting what was passed to grpc, because the failure being
guarded against is precisely a configuration that looks right and secures
nothing.
"""

from __future__ import annotations

import grpc
import pytest
import trustme
from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc

from prepeet_ai.transport import server as transport
from prepeet_ai.transport.server import TLSConfig, TLSConfigError


@pytest.fixture(name="authority")
def _authority() -> trustme.CA:
    return trustme.CA()


def _files(tmp_path, authority: trustme.CA, name: str):
    """Write a leaf certificate for `name` and the authority's root to disk."""
    leaf = authority.issue_cert(name)
    certificate = tmp_path / f"{name}.pem"
    key = tmp_path / f"{name}.key"
    root = tmp_path / "ca.pem"
    leaf.cert_chain_pems[0].write_to_path(certificate)
    leaf.private_key_pem.write_to_path(key)
    authority.cert_pem.write_to_path(root)
    return certificate, key, root


def _probe(channel: grpc.Channel) -> None:
    """Make one call, so the handshake actually happens."""
    stub = intelligence_pb2_grpc.IntelligenceServiceStub(channel)
    stub.ComposeSessionBundle(
        intelligence_pb2.ComposeSessionBundleRequest(), timeout=10
    )


class TestServing:
    def test_a_client_holding_the_authority_completes_the_handshake(
        self, tmp_path, authority
    ) -> None:
        certificate, key, root = _files(tmp_path, authority, "localhost")
        served, port = transport.serve(
            port=0, tls=TLSConfig(cert_file=str(certificate), key_file=str(key))
        )
        try:
            credentials = grpc.ssl_channel_credentials(root.read_bytes())
            with grpc.secure_channel(f"localhost:{port}", credentials) as channel:
                # The call itself is refused for having no session, which is
                # the point: refusal is an answer, and an answer means the
                # transport carried it.
                with pytest.raises(grpc.RpcError) as refused:
                    _probe(channel)
                assert refused.value.code() != grpc.StatusCode.UNAVAILABLE
        finally:
            served.stop(0)

    def test_a_plaintext_client_is_refused_by_a_tls_server(
        self, tmp_path, authority
    ) -> None:
        """The downgrade this whole change exists to prevent."""
        certificate, key, _ = _files(tmp_path, authority, "localhost")
        served, port = transport.serve(
            port=0, tls=TLSConfig(cert_file=str(certificate), key_file=str(key))
        )
        try:
            with grpc.insecure_channel(f"localhost:{port}") as channel:
                with pytest.raises(grpc.RpcError) as refused:
                    _probe(channel)
                assert refused.value.code() == grpc.StatusCode.UNAVAILABLE
        finally:
            served.stop(0)

    def test_a_client_not_holding_the_authority_is_refused(
        self, tmp_path, authority
    ) -> None:
        certificate, key, _ = _files(tmp_path, authority, "localhost")
        served, port = transport.serve(
            port=0, tls=TLSConfig(cert_file=str(certificate), key_file=str(key))
        )
        try:
            stranger = trustme.CA()
            wrong_root = tmp_path / "stranger.pem"
            stranger.cert_pem.write_to_path(wrong_root)
            credentials = grpc.ssl_channel_credentials(wrong_root.read_bytes())
            with grpc.secure_channel(f"localhost:{port}", credentials) as channel:
                with pytest.raises(grpc.RpcError) as refused:
                    _probe(channel)
                assert refused.value.code() == grpc.StatusCode.UNAVAILABLE
        finally:
            served.stop(0)

    def test_no_tls_configuration_still_serves_plaintext(self) -> None:
        """`make dev` must not need a certificate authority."""
        served, port = transport.serve(port=0)
        try:
            with grpc.insecure_channel(f"localhost:{port}") as channel:
                with pytest.raises(grpc.RpcError) as refused:
                    _probe(channel)
                assert refused.value.code() != grpc.StatusCode.UNAVAILABLE
        finally:
            served.stop(0)


class TestMutualTLS:
    def test_a_client_presenting_its_certificate_is_admitted(
        self, tmp_path, authority
    ) -> None:
        certificate, key, root = _files(tmp_path, authority, "localhost")
        client = authority.issue_cert("worker.prepeet.internal")
        served, port = transport.serve(
            port=0,
            tls=TLSConfig(
                cert_file=str(certificate),
                key_file=str(key),
                client_ca_file=str(root),
            ),
        )
        try:
            credentials = grpc.ssl_channel_credentials(
                root_certificates=root.read_bytes(),
                private_key=client.private_key_pem.bytes(),
                certificate_chain=client.cert_chain_pems[0].bytes(),
            )
            with grpc.secure_channel(f"localhost:{port}", credentials) as channel:
                with pytest.raises(grpc.RpcError) as refused:
                    _probe(channel)
                assert refused.value.code() != grpc.StatusCode.UNAVAILABLE
        finally:
            served.stop(0)

    def test_a_client_presenting_nothing_is_refused(
        self, tmp_path, authority
    ) -> None:
        """Client authentication is required, not merely requested."""
        certificate, key, root = _files(tmp_path, authority, "localhost")
        served, port = transport.serve(
            port=0,
            tls=TLSConfig(
                cert_file=str(certificate),
                key_file=str(key),
                client_ca_file=str(root),
            ),
        )
        try:
            credentials = grpc.ssl_channel_credentials(root.read_bytes())
            with grpc.secure_channel(f"localhost:{port}", credentials) as channel:
                with pytest.raises(grpc.RpcError) as refused:
                    _probe(channel)
                assert refused.value.code() == grpc.StatusCode.UNAVAILABLE
        finally:
            served.stop(0)


class TestConfiguration:
    def test_half_a_pair_is_refused(self, tmp_path, authority) -> None:
        certificate, _, _ = _files(tmp_path, authority, "localhost")
        with pytest.raises(TLSConfigError, match="together"):
            TLSConfig(cert_file=str(certificate), key_file="").validate()

    def test_a_client_authority_without_a_server_pair_is_refused(self) -> None:
        with pytest.raises(TLSConfigError, match="server certificate"):
            TLSConfig(client_ca_file="/nonexistent/ca.pem").validate()

    def test_a_missing_file_is_named(self, tmp_path) -> None:
        with pytest.raises(TLSConfigError, match="missing.pem"):
            TLSConfig(
                cert_file=str(tmp_path / "missing.pem"),
                key_file=str(tmp_path / "missing.key"),
            ).read()

    def test_from_env_reads_nothing_when_unset(self) -> None:
        assert TLSConfig.from_env({}).enabled is False

    def test_from_env_reads_the_pair(self) -> None:
        config = TLSConfig.from_env(
            {
                "PREPEET_RPC_TLS_CERT_FILE": "/tls/server.pem",
                "PREPEET_RPC_TLS_KEY_FILE": "/tls/server.key",
                "PREPEET_RPC_TLS_CLIENT_CA_FILE": "/tls/ca.pem",
            }
        )
        assert config.enabled
        assert config.mutual
