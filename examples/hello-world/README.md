# Example Persistence Layer for SAML

This is an example of how you can use `github.com/ucarion/saml` to go through
the SAML flow. It's meant to be the simplest possible app that knows how to set
up and log in with SAML.

You can try this example by running, from the directory where this README is in:

```bash
go run ./...
```

You can now visit localhost:8080 and try out the example app.

## Guided Tour

Here's a suggested approach you can take to trying out this example.

1. First, you'll need to sign up for a free account with [Okta][okta] or
   [OneLogin][onelogin], which are widely-used Identity Providers that support
   free trials.

1. Now we need to set up the connection from the Identity Provider end.

   * If you're using OneLogin, go to your OneLogin administrator page. Go to
     "Applications", and then click "Add App". Choose "SAML Test Connector
     (Advanced)" -- you can use the search to help you find this. Click "Save".

     You've now created a new OneLogin application. We can now paste the
     information from TodoApp into OneLogin. Go to "Configuration", and then put
     `http://localhost:8080/acs` into the OneLogin inputs labeled "Recipient",
     "ACS (Consumer) URL Validator", and "ACS (Consumer) URL".

     Then hit save.

   * If you're using Okta, go to your Okta admin dashboard. Go to
     "Applications", and then click "Add Application". Choose "Create New App"
     at the top right, and then the "SAML 2.0" option. Then click "Create".

     Give the application any name you like, and then click Next.

     Put `http://localhost:8080/acs` into the fields in Okta called "Single sign
     on URL" and "Audience URI (SP Entity ID)". Click "Next".

     Do whatever you like on the "Feedback" page. It doesn't matter. Click
     "Finish".

1. Next we need to set up SAML from the Service Provider end. In this case, the
   demo app is the service provider.

   * If you're using OneLogin, click on the "More Actions" dropdown at the top
     right of the edit page for the OneLogin app you just created. Click on the
     "SAML Metadata" option with a download icon.

   * If you're using Okta, click on the "Sign On" tab of your application. A
     yellow-bordered box contains a link labeled "Identity Provider metadata".
     Click on that link. Save the page. Your browser may ask what format you
     want to save it in -- choose "XML", not "Web page".

     You'll also need to assign the app to yourself at this time. Go to
     "Assignments", click "Assign" and then "Assign People". Find your Okta user
     in the list of users, click "Assign", and then "Save and Go Back", then
     "Done".

1. Go to the directory where you downloaded that metadata file. Assuming you
   saved it into a file called `metadata.xml`, now run:

   ```bash
   curl http://localhost:8080/setup -d @metadata.xml
   ```

1. You can now try logging in with SAML. Visit:

   ```text
   http://localhost:8080/initiate
   ```

   With your *browser*, not cURL. You should be redirected to your identity
   provider, and then sent back to a page with the URL like:

   ```text
   http://localhost:8080/acs
   ```

   The page's content should also show you some details about yourself from your
   identity provider. That's information that the demo app extracted from the
   SAML login flow.

There are some additional follow-up tests you could try. For instance, try
visiting:

```text
http://localhost:8080/initiate?relay_state=foobar
```

You'll find that you get directed back to the `.../acs` URL, but with the
`relay_state` parameter populated. A "Relay State" is how you typically do
deeplinking with SAML.

As a last little test, you can try logging into the demo app directly from your
Identity Provider, instead of relying on the `.../initiate` URL. Find the app
you created in your identity provider, and click on it. You may need to exit the
"Admin" view of your Identity Provider, so that it logs you in instead of trying
to edit the connection details.
