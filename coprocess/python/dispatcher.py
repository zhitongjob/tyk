from glob import glob
from os import getcwd, chdir, path
import sys

import tyk
from tyk.middleware import TykMiddleware
from tyk.object import TykCoProcessObject
from tyk.event import TykEvent, TykEventHandler

from gateway import TykGateway as tyk

class TykDispatcher:
    '''A simple dispatcher'''

    def __init__(self, middleware_path, event_handler_path, bundle_path):
        tyk.log( "Initializing dispatcher", "info" )

        self.event_handler_path = path.join(event_handler_path, '*.py')
        self.event_handlers = {}
        self.load_event_handlers()

        self.middleware_path = path.join(middleware_path, '*.py')
        self.bundle_path = bundle_path

        self.bundles = []
        self.hook_table = {}
        self.load_middlewares()

    def get_modules(self, the_path):
        files = glob(the_path)
        files = [ path.basename( f.replace('.py', '') ) for f in files ]
        return files

    def find_bundle(self, bundle_id):
        found = None
        for bundle in self.bundles:
            if bundle.bundle_id == bundle_id:
                found = bundle
                break
        return found

    def load_bundle(self, bundle_path):
        path_splits = bundle_path.split('/')
        bundle_id = path_splits[-1]
        bundle = self.find_bundle(bundle_id)
        if not bundle:
            bundle = TykMiddleware(bundle_id)
            self.bundles.append(bundle)
            self.update_hook_table(with_bundle=bundle)

            return
        self.update_hook_table(with_bundle=bundle)


    def load_middlewares(self):
        tyk.log( "Loading middlewares.", "debug" )

    def purge_middlewares(self):
        self.middlewares = []

    def init_hook_table(self):
        new_hook_table = {}
        for bundle in self.bundles:
            api_id = bundle.api_id
            hooks = {}
            for hook_type in bundle.handlers:
                for handler in bundle.handlers[hook_type]:
                    handler.middleware = bundle
                    hooks[handler.name] = handler
            new_hook_table[api_id] = hooks
        self.hook_table = new_hook_table

    def update_hook_table(self, with_bundle=None):
        new_hook_table = {}
        # Disable any previous bundle associated with an API:
        if with_bundle:
            # First check if this API exists in the hook table:
            the_hooks = []
            if with_bundle.api_id in self.hook_table:
                the_hooks = self.hook_table[with_bundle.api_id]
            if len(the_hooks) > 0:
                # Pick the first hook and get the current bundle:
                bundle_in_use = list(the_hooks.values())[0].middleware
                # If the bundle is already in use, skip the hook table update:
                if bundle_in_use.bundle_id == with_bundle.bundle_id:
                    return
            the_hooks = with_bundle.build_hooks()
            self.hook_table[with_bundle.api_id] = the_hooks

    def find_hook_by_type_and_name(self, hook_type, hook_name):
        found_middleware, matching_hook_handler = None, None
        for middleware in self.middlewares:
            if hook_type in middleware.handlers:
                for handler in middleware.handlers[hook_type]:
                    if handler.name == hook_name:
                        found_middleware = middleware
                        matching_hook_handler = handler
        return found_middleware, matching_hook_handler

    def find_hook_by_name(self, hook_name):
        hook_handler, middleware = None, None
        if hook_name in self.hook_table:
            hook_handler = self.hook_table[hook_name]
            middleware = hook_handler.middleware
        return middleware, hook_handler

    def find_hook(self, api_id, hook_name):
        hooks = self.hook_table[api_id]
        if hook_name not in hooks:
            return None
        hook = hooks[hook_name]
        return hook.middleware, hook

    def dispatch_hook(self, object_msg):
        try:
            object = TykCoProcessObject(object_msg)
            api_id = object.spec['APIID']
            middleware, hook_handler = self.find_hook(api_id, object.hook_name)
            if hook_handler:
                object = middleware.process(hook_handler, object)
            else:
                tyk.log( "Can't dispatch '{0}', hook is not defined.".format(object.hook_name), "error")
            return object.dump()
        except:
            tyk.log_error( "Can't dispatch, error:" )
            return object_msg

    def purge_event_handlers(self):
        tyk.log( "Purging event handlers.", "debug" )
        self.event_handlers = []

    def load_event_handlers(self):
        tyk.log( "Loading event handlers.", "debug" )
        for module_name in self.get_modules(self.event_handler_path):
            event_handlers = TykEventHandler.from_module(module_name)
            for event_handler in event_handlers:
                self.event_handlers[event_handler.name] = event_handler

    def find_event_handler(self, handler_name):
        handler = None
        if handler_name in self.event_handlers:
            handler = self.event_handlers[handler_name]
        return handler

    def dispatch_event(self, event_json):
        try:
            event = TykEvent(event_json)
            event_handler = self.find_event_handler(event.handler_name)
            if event_handler:
                event_handler.process(event)
        except:
            tyk.log_error( "Can't dispatch, error:")

    def reload(self):
        tyk.log( "Reloading event handlers and middlewares.", "info" )

        # self.purge_event_handlers()
        # self.load_event_handlers()

        # self.purge_middlewares()
        # self.load_middlewares()
