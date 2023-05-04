from steps.cluster import Cluster


def before_all(context):
    context.cluster = Cluster()
