from behave import given, then, when, step
from steps.steps import apply_resource
import polling2


provider_template = '''
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: {name}
spec:
    package: {repo}/{name}:{version}
'''


@given(u'provider {name}:{version} is running in cluster')
@given(u'provider {repo}/{name}:{version} is running in cluster')
def provider_get_installed(context, name, version, repo="xpkg.upbound.io/upbound"):
    provider_raw = provider_template.format(name=name, version=version, repo=repo)
    provider_yaml = apply_resource(context.cluster, provider_raw)
    context.cluster.wait_for_jq_match("providers.pkg.crossplane.io", name, '.status.conditions[] | select(.type=="Healthy").status', 'True')
    context.cluster.wait_for_jq_match("providers.pkg.crossplane.io", name, '.status.conditions[] | select(.type=="Installed").status', 'True')
    # apply_resource(dummy_server)
    # polling2.poll(lambda: json.loads(
    #     cluster.get_resource_by_jq("deployment", "server-dummy", "crossplane-system", '.status.conditions[] | select(.type=="Available").status')) == 'True',
    #               step=5, timeout=800, ignore_exceptions=(json.JSONDecodeError,))
    # apply_resource(dummy_service)
    # apply_resource(provider_configs[name])
